package main

// opensnitch_server.go — UIService gRPC server for Interactive mode.
//
// Architecture recap:
//   opensnitchd = gRPC CLIENT  →  dials unix:///tmp/osui.sock
//   This server = gRPC SERVER  →  listens on /tmp/osui.sock
//
// For each new connection opensnitchd calls AskRule().
// We hold that call for up to 30s while the DECK shows the prompt.
// User hits [a] or [d] → verdict sent → AskRule returns → connection proceeds.
// Timeout (30s) → allow (Interactive default).
//
// Paranoid mode doesn't use this server — DefaultAction=deny + rules do the work.
// Watch mode doesn't use this server — DefaultAction=allow, everything passes.

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"google.golang.org/grpc"

	protocol "char-gen-cli/opensnitch/daemon/ui/protocol"
)

// ── Mode ─────────────────────────────────────────────────────────────────────

type SnitchMode int

const (
	ModeWatch       SnitchMode = iota // allow all, log all
	ModeInteractive                   // 30s hold → allow on timeout, learns from history
	ModeParanoid                      // deny all, rules-only allow
)

func (m SnitchMode) String() string {
	switch m {
	case ModeWatch:
		return "WATCH"
	case ModeInteractive:
		return "INTERACTIVE"
	case ModeParanoid:
		return "PARANOID"
	}
	return "UNKNOWN"
}

func (m SnitchMode) Label() string {
	switch m {
	case ModeWatch:
		return "👁  WATCH — log only"
	case ModeInteractive:
		return "⏸  INTERACTIVE — 30s prompt"
	case ModeParanoid:
		return "🔒 PARANOID — deny all"
	}
	return ""
}

// ── History ──────────────────────────────────────────────────────────────────

// SnitchHistory records past user verdicts for Interactive auto-apply.
type SnitchHistory struct {
	mu      sync.RWMutex
	entries map[string]string // process_path → "allow" | "deny"
}

func newSnitchHistory() *SnitchHistory {
	return &SnitchHistory{entries: make(map[string]string)}
}

func (h *SnitchHistory) Lookup(processPath string) (action string, found bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	v, ok := h.entries[processPath]
	return v, ok
}

func (h *SnitchHistory) Record(processPath, action string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.entries[processPath] = action
}

func (h *SnitchHistory) All() map[string]string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make(map[string]string, len(h.entries))
	for k, v := range h.entries {
		out[k] = v
	}
	return out
}

// ── Pending connection ────────────────────────────────────────────────────────

const snitchHoldTimeout = 30 * time.Second

// SnitchPending is a connection waiting for user verdict in Interactive mode.
type SnitchPending struct {
	ID        string
	Process   string // basename
	FullPath  string
	DstHost   string
	DstIP     string
	DstPort   uint32
	Proto     string
	CreatedAt time.Time
	verdict   chan string
}

func (p *SnitchPending) Remaining() time.Duration {
	r := snitchHoldTimeout - time.Since(p.CreatedAt)
	if r < 0 {
		return 0
	}
	return r
}

// ── Messages ──────────────────────────────────────────────────────────────────

type SnitchPendingMsg struct{ Pending *SnitchPending }
type SnitchVerdictSentMsg struct{ ID, Action string }
type SnitchModeChangedMsg struct{ Mode SnitchMode }
type SnitchCountdownTickMsg struct{}

// ── gRPC server ───────────────────────────────────────────────────────────────

type snitchUIServer struct {
	protocol.UnimplementedUIServer
	pendingCh chan *SnitchPending
	history   *SnitchHistory
}

func (s *snitchUIServer) Ping(_ context.Context, _ *protocol.PingRequest) (*protocol.PingReply, error) {
	return &protocol.PingReply{}, nil
}

func (s *snitchUIServer) Subscribe(_ context.Context, cfg *protocol.ClientConfig) (*protocol.ClientConfig, error) {
	return cfg, nil
}

func (s *snitchUIServer) Notifications(stream grpc.BidiStreamingServer[protocol.NotificationReply, protocol.Notification]) error {
	<-stream.Context().Done()
	return nil
}

func (s *snitchUIServer) PostAlert(_ context.Context, _ *protocol.Alert) (*protocol.MsgResponse, error) {
	return &protocol.MsgResponse{}, nil
}

func (s *snitchUIServer) AskRule(ctx context.Context, conn *protocol.Connection) (*protocol.Rule, error) {
	processPath := conn.GetProcessPath()

	// Check history first — if we've seen this process before, auto-apply
	if action, found := s.history.Lookup(processPath); found {
		return snitchRule(action), nil
	}

	id := fmt.Sprintf("%d", time.Now().UnixNano())
	pending := &SnitchPending{
		ID:        id,
		Process:   filepath.Base(processPath),
		FullPath:  processPath,
		DstHost:   conn.GetDstHost(),
		DstIP:     conn.GetDstIp(),
		DstPort:   conn.GetDstPort(),
		Proto:     conn.GetProtocol(),
		CreatedAt: time.Now(),
		verdict:   make(chan string, 1),
	}

	// Queue for DECK display; if queue full, allow through
	select {
	case s.pendingCh <- pending:
	default:
		return snitchRule("allow"), nil
	}

	select {
	case v := <-pending.verdict:
		return snitchRule(v), nil
	case <-time.After(snitchHoldTimeout):
		return snitchRule("allow"), nil // Interactive timeout = allow
	case <-ctx.Done():
		return snitchRule("allow"), nil
	}
}

func snitchRule(action string) *protocol.Rule {
	return &protocol.Rule{
		Action:   action,
		Duration: "once",
		Enabled:  true,
		Name:     "interactive-verdict",
	}
}

// ── Server lifecycle globals ──────────────────────────────────────────────────

var (
	gSnitchServer *snitchUIServer
	gGRPCServer   *grpc.Server
	gSnitchMode   = ModeWatch
	gHistory      = newSnitchHistory()
)

const snitchSocketPath = "/tmp/osui.sock"

func startSnitchGRPCServer() error {
	stopSnitchGRPCServer()

	os.Remove(snitchSocketPath)
	ln, err := net.Listen("unix", snitchSocketPath)
	if err != nil {
		return fmt.Errorf("listen %s: %w", snitchSocketPath, err)
	}

	srv := &snitchUIServer{
		pendingCh: make(chan *SnitchPending, 8),
		history:   gHistory,
	}
	gSnitchServer = srv

	gs := grpc.NewServer()
	protocol.RegisterUIServer(gs, srv)
	gGRPCServer = gs

	go gs.Serve(ln)
	NexusLog("INFO", "snitch", "gRPC UI server started (Interactive mode)")
	return nil
}

func stopSnitchGRPCServer() {
	if gGRPCServer != nil {
		gGRPCServer.Stop()
		gGRPCServer = nil
	}
	gSnitchServer = nil
	os.Remove(snitchSocketPath)
}

// ── Bubble Tea commands ───────────────────────────────────────────────────────

// SnitchSetModeCmd switches mode, restarts opensnitchd with the right config.
func SnitchSetModeCmd(mode SnitchMode) tea.Cmd {
	return func() tea.Msg {
		// Stop daemon
		exec.Command("pkill", "-x", "opensnitchd").Run()
		time.Sleep(400 * time.Millisecond)

		gSnitchMode = mode

		switch mode {
		case ModeWatch:
			stopSnitchGRPCServer()
			writeModeConfig("allow")

		case ModeInteractive:
			if err := startSnitchGRPCServer(); err != nil {
				NexusLog("ERROR", "snitch", err.Error())
			}
			writeModeConfig("allow") // gRPC intercepts before DefaultAction is reached

		case ModeParanoid:
			stopSnitchGRPCServer() // no gRPC needed — rules control everything
			writeModeConfig("deny")
		}

		// Restart daemon with mode config
		rDir := rulesDir()
		os.MkdirAll(rDir, 0755)
		cmd := exec.Command(snitchBin,
			"-config-file", modeConfigPath(),
			"-rules-path", rDir,
			"-fw-config-file", "/etc/opensnitchd/system-fw.json",
		)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			NexusLog("ERROR", "snitch", fmt.Sprintf("restart failed: %v", err))
			return SnitchModeChangedMsg{Mode: mode}
		}
		cmd.Process.Release()
		NexusLog("INFO", "snitch", fmt.Sprintf("mode switched to %s", mode))
		return SnitchModeChangedMsg{Mode: mode}
	}
}

// SnitchWatchPendingCmd blocks until the next pending connection arrives.
// Must be re-issued after each SnitchPendingMsg is handled.
func SnitchWatchPendingCmd() tea.Cmd {
	return func() tea.Msg {
		if gSnitchServer == nil {
			// Block forever — no server running, no pending connections
			select {}
		}
		pending := <-gSnitchServer.pendingCh
		return SnitchPendingMsg{Pending: pending}
	}
}

// SnitchSendVerdictCmd sends user verdict to the waiting AskRule gRPC call.
// If remember=true, records the decision in history for auto-apply next time.
func SnitchSendVerdictCmd(pending *SnitchPending, action string, remember bool) tea.Cmd {
	return func() tea.Msg {
		if pending == nil {
			return nil
		}
		if remember {
			gHistory.Record(pending.FullPath, action)
		}
		select {
		case pending.verdict <- action:
		default:
		}
		return SnitchVerdictSentMsg{ID: pending.ID, Action: action}
	}
}

// SnitchCountdownCmd fires every second while a pending connection is active.
func SnitchCountdownCmd() tea.Cmd {
	return tea.Tick(time.Second, func(_ time.Time) tea.Msg {
		return SnitchCountdownTickMsg{}
	})
}

// ── Mode config helpers ───────────────────────────────────────────────────────

func modeConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opensnitch", fmt.Sprintf("mode-%s.json", strings.ToLower(gSnitchMode.String())))
}

func writeModeConfig(defaultAction string) {
	home, _ := os.UserHomeDir()
	dir := filepath.Join(home, ".config", "opensnitch")
	os.MkdirAll(dir, 0755)

	name := strings.ToLower(gSnitchMode.String())
	path := filepath.Join(dir, fmt.Sprintf("mode-%s.json", name))

	config := fmt.Sprintf(`{
    "Server": {
        "Address": "unix:///tmp/osui.sock",
        "Authentication": {"Type": "simple", "TLSOptions": {"CACert": "", "ServerCert": "", "ClientCert": "", "ClientKey": "", "SkipVerify": false, "ClientAuthType": "no-client-cert"}},
        "LogFile": "/var/log/opensnitchd.log",
        "Loggers": [{"Name": "syslog", "Server": "127.0.0.1:514", "Protocol": "udp", "Format": "rfc5424", "Tag": "opensnitchd"}]
    },
    "DefaultAction": "%s",
    "DefaultDuration": "always",
    "InterceptUnknown": true,
    "ProcMonitorMethod": "proc",
    "LogLevel": 1,
    "LogUTC": false,
    "LogMicro": false,
    "Firewall": "nftables",
    "FwOptions": {"ConfigPath": "/etc/opensnitchd/system-fw.json", "MonitorInterval": "15s", "QueueBypass": true},
    "Rules": {"Path": "%s/", "EnableChecksums": false},
    "Stats": {"MaxEvents": 250, "MaxStats": 25, "Workers": 6},
    "Internal": {"GCPercent": 100, "FlushConnsOnStart": true}
}`, defaultAction, rulesDir())

	os.WriteFile(path, []byte(config), 0644)
}
