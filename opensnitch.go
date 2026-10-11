package main

// opensnitch.go — OpenSnitch host-native integration
// Manages opensnitchd directly (no container).
// Binary: /usr/local/bin/opensnitchd
// Rules:  ~/.config/opensnitch/rules/
// Log:    /var/log/opensnitchd.log
//
// NOTE: opensnitchd uses an inverted gRPC architecture — opensnitchd is the
// CLIENT, the UI must run a gRPC SERVER. Full gRPC server implementation is
// pending. Current approach: poll log + pgrep for status.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	snitchBin    = "/usr/local/bin/opensnitchd"
	snitchLog    = "/var/log/opensnitchd.log"
	snitchSyslog = "/var/log/messages"
	snitchConfig = "/etc/opensnitchd/default-config.json"
	snitchSocket = "/tmp/osui.sock"
)

// ── Data types ───────────────────────────────────────────────────────────────

type SnitchEvent struct {
	Time    time.Time
	Process string
	Dst     string // IP address
	DstHost string // resolved hostname (may be empty)
	Port    string
	Proto   string
	Action  string // "intercepted" | "allowed" | "denied" | "unseen"
}

type SnitchRule struct {
	Created  string         `json:"created"`
	Updated  string         `json:"updated"`
	Name     string         `json:"name"`
	Enabled  bool           `json:"enabled"`
	Action   string         `json:"action"` // "allow" | "deny"
	Duration string         `json:"duration"`
	Operator SnitchOperator `json:"operator"`
}

type SnitchOperator struct {
	Type      string `json:"type"`
	Sensitive bool   `json:"sensitive"`
	Operand   string `json:"operand"`
	Data      string `json:"data"`
}

// ── Messages ─────────────────────────────────────────────────────────────────

type SnitchUpdateMsg struct {
	Events []SnitchEvent
	Active bool
}

type SnitchRuleWrittenMsg struct {
	Rule   SnitchRule
	Action string
	Err    error
}

type SnitchDaemonMsg struct {
	Action string // "start" | "stop"
	Err    error
}

// ── Start / Stop ──────────────────────────────────────────────────────────────

func SnitchStartCmd() tea.Cmd {
	return func() tea.Msg {
		if isOsRunning() {
			return SnitchDaemonMsg{Action: "start"}
		}

		rDir := rulesDir()
		if err := os.MkdirAll(rDir, 0755); err != nil {
			return SnitchDaemonMsg{Action: "start", Err: err}
		}

		cmd := exec.Command(snitchBin,
			"-config-file", snitchConfig,
			"-rules-path", rDir,
			"-fw-config-file", "/etc/opensnitchd/system-fw.json",
		)
		cmd.Stdout = nil
		cmd.Stderr = nil
		if err := cmd.Start(); err != nil {
			return SnitchDaemonMsg{Action: "start", Err: err}
		}
		cmd.Process.Release()

		NexusLog("INFO", "snitch", "opensnitchd started")
		return SnitchDaemonMsg{Action: "start"}
	}
}

func SnitchStopCmd() tea.Cmd {
	return func() tea.Msg {
		exec.Command("pkill", "-x", "opensnitchd").Run()
		NexusLog("INFO", "snitch", "opensnitchd stopped")
		return SnitchDaemonMsg{Action: "stop"}
	}
}

// ── Polling ──────────────────────────────────────────────────────────────────

func SnitchHeartbeat() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return fetchSnitchData()
	})
}

func fetchSnitchData() SnitchUpdateMsg {
	msg := SnitchUpdateMsg{Active: isOsRunning()}

	if !msg.Active {
		return msg
	}

	// Try syslog first (requires Loggers syslog config in opensnitchd)
	if out, err := exec.Command("tail", "-n", "200", snitchSyslog).Output(); err == nil {
		events := parseSyslog(string(out))
		if len(events) > 0 {
			msg.Events = events
			return msg
		}
	}

	// Fall back to opensnitchd log file
	if out, err := exec.Command("tail", "-n", "100", snitchLog).Output(); err == nil {
		msg.Events = parseSnitchLog(string(out))
	}

	return msg
}

// parseSyslog extracts opensnitchd CONNECTION events from /var/log/messages.
// Format: ... opensnitch[pid]: <N>1 timestamp  pid CONNECTION - [KEY="val" ...]
func parseSyslog(raw string) []SnitchEvent {
	var events []SnitchEvent
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "opensnitch") || !strings.Contains(line, "CONNECTION") {
			continue
		}
		e := SnitchEvent{Time: time.Now(), Action: "intercepted"}
		e.Dst = syslogField(line, "DST")
		e.DstHost = syslogField(line, "DSTHOST")
		e.Port = syslogField(line, "DPT")
		e.Proto = syslogField(line, "PROTO")
		fullPath := syslogField(line, "PATH")
		e.Process = filepath.Base(fullPath)

		// Resolve verdict against rules
		e.Action = resolveVerdict(e)

		if e.Dst != "" || e.Process != "" {
			events = append(events, e)
		}
	}
	if len(events) > 20 {
		events = events[len(events)-20:]
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}

// syslogField extracts KEY="value" from an opensnitchd syslog line.
func syslogField(line, key string) string {
	needle := key + `="`
	idx := strings.Index(line, needle)
	if idx < 0 {
		return ""
	}
	rest := line[idx+len(needle):]
	end := strings.IndexByte(rest, '"')
	if end < 0 {
		return rest
	}
	return rest[:end]
}

func isOsRunning() bool {
	// Check for running process
	out, err := exec.Command("pgrep", "-x", "opensnitchd").Output()
	return err == nil && len(strings.TrimSpace(string(out))) > 0
}

func IsOsRunning() bool { return isOsRunning() }

// parseSnitchLog parses opensnitchd log lines into SnitchEvents.
func parseSnitchLog(raw string) []SnitchEvent {
	var events []SnitchEvent
	scanner := bufio.NewScanner(strings.NewReader(raw))
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.Contains(line, "->") && !strings.Contains(line, "connection") {
			continue
		}
		e := SnitchEvent{Time: time.Now(), Action: "intercepted"}

		if idx := strings.Index(line, "process:"); idx >= 0 {
			rest := line[idx+8:]
			if end := strings.IndexAny(rest, " ,\t"); end > 0 {
				e.Process = strings.TrimSpace(rest[:end])
			}
		}
		if idx := strings.Index(line, "->"); idx >= 0 {
			rest := strings.TrimSpace(line[idx+2:])
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				hp := strings.SplitN(parts[0], ":", 2)
				e.Dst = hp[0]
				if len(hp) == 2 {
					e.Port = hp[1]
				}
			}
		}
		if strings.Contains(line, "allow") {
			e.Action = "allowed"
		} else if strings.Contains(line, "deny") || strings.Contains(line, "block") {
			e.Action = "denied"
		}
		if e.Dst != "" || e.Process != "" {
			events = append(events, e)
		}
	}
	if len(events) > 20 {
		events = events[len(events)-20:]
	}
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
	return events
}

// ── Rule writing ─────────────────────────────────────────────────────────────

func rulesDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "opensnitch", "rules")
}

// SnitchReportCmd launches the report script in a new tmux window.
func SnitchReportCmd() tea.Cmd {
	return func() tea.Msg {
		script := os.ExpandEnv("$HOME/claude/nexus-network-stack/scripts/nexus-snitch-report.sh")
		cmd := exec.Command("tmux", "new-window", "-n", "snitch-report",
			"sh", "-c", script+" --today; echo ''; echo 'Press any key to close...'; read -n1")
		cmd.Run()
		return nil
	}
}

func SnitchAllowCmd(event SnitchEvent) tea.Cmd {
	return func() tea.Msg { return writeSnitchRule(event, "allow") }
}

func SnitchDenyCmd(event SnitchEvent) tea.Cmd {
	return func() tea.Msg { return writeSnitchRule(event, "deny") }
}

// resolveVerdict cross-references an event against the rules directory.
// Returns "allowed", "denied", or "unseen" (no matching rule — unclassified).
func resolveVerdict(e SnitchEvent) string {
	if e.Action == "allowed" || e.Action == "denied" {
		return e.Action
	}
	dir := rulesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "intercepted"
	}
	procBase := strings.ToLower(filepath.Base(e.Process))
	dstIP := strings.ToLower(e.Dst)
	dstHost := strings.ToLower(e.DstHost)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var r SnitchRule
		if err := json.Unmarshal(data, &r); err != nil || !r.Enabled {
			continue
		}
		d := strings.ToLower(r.Operator.Data)
		if d == "" {
			continue
		}
		switch r.Operator.Operand {
		case "process.path":
			if strings.Contains(d, procBase) || strings.Contains(procBase, d) {
				return r.Action
			}
		case "dest.host":
			// match against resolved hostname (DSTHOST) or IP
			if dstHost == d || strings.HasSuffix(dstHost, "."+d) ||
				dstIP == d || strings.HasSuffix(dstIP, "."+d) {
				return r.Action
			}
		case "dest.ip":
			if dstIP == d {
				return r.Action
			}
		default:
			if strings.Contains(dstIP, d) || strings.Contains(dstHost, d) || strings.Contains(procBase, d) {
				return r.Action
			}
		}
	}
	// Export uses "unseen", live feed uses "intercepted"
	if e.Action == "allowed" || e.Action == "denied" {
		return e.Action
	}
	return "intercepted"
}

func SnitchExportCmd(events []SnitchEvent) tea.Cmd {
	return func() tea.Msg {
		if len(events) == 0 {
			return nil
		}

		exportDir := os.ExpandEnv("$HOME/claude/nexus-network-stack/exports")
		os.MkdirAll(exportDir, 0755)

		timestamp := time.Now().Format("20060102-150405")
		filename := fmt.Sprintf("snitch-audit-%s.md", timestamp)
		path := filepath.Join(exportDir, filename)

		f, err := os.Create(path)
		if err != nil {
			return nil
		}

		var allowed, denied, unseen int
		type row struct {
			e       SnitchEvent
			verdict string
		}
		var rows []row
		for _, e := range events {
			v := resolveVerdict(e)
			if v == "intercepted" {
				v = "unseen"
			}
			rows = append(rows, row{e, v})
			switch v {
			case "allowed":
				allowed++
			case "denied":
				denied++
			default:
				unseen++
			}
		}

		w := bufio.NewWriter(f)
		fmt.Fprintf(w, "# NeXuS Security Audit\n\n")
		fmt.Fprintf(w, "> Generated: %s\n\n", time.Now().Format(time.RFC1123))
		fmt.Fprintf(w, "| Stat | Count |\n|------|-------|\n")
		fmt.Fprintf(w, "| Total | %d |\n", len(rows))
		fmt.Fprintf(w, "| Allowed | %d |\n", allowed)
		fmt.Fprintf(w, "| Denied | %d |\n", denied)
		fmt.Fprintf(w, "| Unseen (no rule) | %d |\n\n", unseen)

		fmt.Fprintf(w, "---\n\n")
		fmt.Fprintf(w, "| TIME | PROCESS | DESTINATION | PORT | PROTO | VERDICT |\n")
		fmt.Fprintf(w, "|------|---------|-------------|------|-------|---------|\n")

		for _, r := range rows {
			var label string
			switch r.verdict {
			case "allowed":
				label = "✅ ALLOWED"
			case "denied":
				label = "🚫 DENIED"
			default:
				label = "👁  UNSEEN"
			}
			fmt.Fprintf(w, "| %s | `%s` | `%s` | %s | %s | %s |\n",
				r.e.Time.Format("15:04:05"),
				r.e.Process,
				r.e.Dst,
				r.e.Port,
				r.e.Proto,
				label,
			)
		}

		// Unseen section — actionable list for rule creation
		if unseen > 0 {
			fmt.Fprintf(w, "\n---\n\n## Unseen Connections — No Rule Exists\n\n")
			fmt.Fprintf(w, "_These processes have no allow/deny rule. Review and classify._\n\n")
			fmt.Fprintf(w, "| PROCESS | DESTINATION | PORT |\n|---------|-------------|------|\n")
			seen := map[string]bool{}
			for _, r := range rows {
				if r.verdict != "unseen" {
					continue
				}
				key := r.e.Process + "|" + r.e.Dst
				if seen[key] {
					continue
				}
				seen[key] = true
				fmt.Fprintf(w, "| `%s` | `%s` | %s |\n", r.e.Process, r.e.Dst, r.e.Port)
			}
		}

		fmt.Fprintf(w, "\n---\n\n_meWEwowow — Sane • Simple • Secure • Stealthy • Beautiful_\n")
		w.Flush()
		f.Close()

		// Open in new tmux window with glow if available
		viewer := "cat"
		if _, err := exec.LookPath("glow"); err == nil {
			viewer = "glow"
		}
		exec.Command("tmux", "new-window", "-n", "audit",
			"sh", "-c", fmt.Sprintf("%s %q; echo; echo 'Press any key...'; read -n1", viewer, path)).Run()

		NexusLog("INFO", "snitch", "audit exported: "+filename)
		return nil
	}
}

func writeSnitchRule(event SnitchEvent, action string) SnitchRuleWrittenMsg {
	if err := os.MkdirAll(rulesDir(), 0755); err != nil {
		return SnitchRuleWrittenMsg{Action: action, Err: err}
	}

	name := sanitizeRuleName(fmt.Sprintf("%s-%s-%s", action, filepath.Base(event.Process), event.Dst))
	now := time.Now().UTC().Format(time.RFC3339)

	operand := "dest.ip"
	data := event.Dst
	if event.Process != "" {
		operand = "process.path"
		data = event.Process
	}

	rule := SnitchRule{
		Created:  now,
		Updated:  now,
		Name:     name,
		Enabled:  true,
		Action:   action,
		Duration: "always",
		Operator: SnitchOperator{
			Type:      "simple",
			Sensitive: false,
			Operand:   operand,
			Data:      data,
		},
	}

	path := filepath.Join(rulesDir(), name+".json")
	f, err := os.Create(path)
	if err != nil {
		return SnitchRuleWrittenMsg{Rule: rule, Action: action, Err: err}
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(rule); err != nil {
		return SnitchRuleWrittenMsg{Rule: rule, Action: action, Err: err}
	}

	NexusLog("INFO", "snitch", fmt.Sprintf("rule written: %s %s → %s", action, operand, data))
	return SnitchRuleWrittenMsg{Rule: rule, Action: action}
}

func sanitizeRuleName(s string) string {
	var b strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') ||
			(c >= '0' && c <= '9') || c == '-' || c == '_' {
			b.WriteRune(c)
		} else {
			b.WriteRune('-')
		}
	}
	r := b.String()
	if len(r) > 64 {
		r = r[:64]
	}
	return r
}
