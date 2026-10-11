package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"char-gen-cli/ghostgate"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// nexusHome returns the project root directory.
// Portable: set NEXUS_HOME env var, or it walks up from the binary.
func nexusHome() string {
	if h := os.Getenv("NEXUS_HOME"); h != "" {
		return h
	}
	exe, err := os.Executable()
	if err == nil {
		// binary lives in nexus-network-stack/deck/ → parent is project root
		return filepath.Dir(filepath.Dir(exe))
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "claude", "nexus-network-stack")
}

// ── Unified service item ────────────────────────────────────────────────────

// HydraItem covers both system services (ghost gate, ollama) and podman containers.
type HydraItem struct {
	Name   string
	Status string
	Kind   string // "ghost" | "ollama" | "container"
}

// AllItems returns system services + containers as a flat navigable list.
func (h HydraStatus) AllItems() []HydraItem {
	var items []HydraItem

	gateStatus := "Inactive"
	if h.GhostGate {
		gateStatus = "Active"
	}
	items = append(items, HydraItem{Name: "Ghost Gate", Status: gateStatus, Kind: "ghost"})

	ollamaStatus := "Stopped"
	if h.Ports["ollama"] {
		ollamaStatus = "Running"
	}
	items = append(items, HydraItem{Name: "Ollama AI", Status: ollamaStatus, Kind: "ollama"})

	for _, c := range h.Containers {
		items = append(items, HydraItem{Name: c.Name, Status: c.Status, Kind: "container"})
	}
	return items
}

// ── Commands ────────────────────────────────────────────────────────────────

// ContainerActionMsg is returned after any start/stop action completes.
type ContainerActionMsg struct {
	Name   string
	Action string // "start" or "stop"
	Err    error
}

func HydraItemStartCmd(item HydraItem) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch item.Kind {
		case "ghost":
			nftFile := filepath.Join(nexusHome(), "config", "ghost-gate.nft")
			err = exec.Command("sh", "-c",
				"doas nft -f "+nftFile+" && touch /tmp/nexus_ghost.lock").Run()
		case "ollama":
			err = exec.Command("sh", "-c",
				"nohup ollama serve >/tmp/ollama.log 2>&1 &").Run()
		case "container":
			err = exec.Command("podman", "start", item.Name).Run()
		}
		return ContainerActionMsg{Name: item.Name, Action: "start", Err: err}
	}
}

func HydraItemStopCmd(item HydraItem) tea.Cmd {
	return func() tea.Msg {
		var err error
		switch item.Kind {
		case "ghost":
			// Delete only the ghost_gate table — leaves other nft rules intact
			exec.Command("sh", "-c",
				"doas nft delete table inet ghost_gate; rm -f /tmp/nexus_ghost.lock ~/.nexus/gate_state.json").Run()
		case "ollama":
			err = exec.Command("pkill", "-f", "ollama serve").Run()
		case "container":
			err = exec.Command("podman", "stop", item.Name).Run()
		}
		return ContainerActionMsg{Name: item.Name, Action: "stop", Err: err}
	}
}

// ── Polling ─────────────────────────────────────────────────────────────────

func pollHydraStatus() HydraStatus {
	s := HydraStatus{
		Profile:  readHydraProfile(),
		Tier:     detectHydraTier(),
		Ports:    make(map[string]bool),
		LastPoll: time.Now(),
	}

	out, err := exec.Command("podman", "ps", "--format", "{{.Names}}\t{{.Status}}").Output()
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			parts := strings.SplitN(line, "\t", 2)
			name, status := parts[0], ""
			if len(parts) == 2 {
				status = parts[1]
			}
			if strings.ContainsAny(name, "_") &&
				(strings.Contains(name, "nexus") || strings.Contains(name, "tor_") ||
					strings.Contains(name, "i2p") || strings.Contains(name, "mesh") ||
					strings.Contains(name, "bridge")) {
				s.Containers = append(s.Containers, HydraContainer{Name: name, Status: status})
			}
		}
	}

	s.Ports["tor"]     = portOpen("127.0.0.1:9050")
	s.Ports["i2p"]     = portOpen("127.0.0.1:4444")
	s.Ports["haproxy"] = portOpen("127.0.0.1:8080")
	s.Ports["privoxy"] = portOpen("127.0.0.1:8118")
	s.Ports["ollama"]  = portOpen("127.0.0.1:11434")
	s.GhostGate        = ghostgate.IsActive()

	return s
}

func portOpen(addr string) bool {
	c, err := net.DialTimeout("tcp", addr, 500*time.Millisecond)
	if err != nil {
		return false
	}
	c.Close()
	return true
}

func readHydraProfile() string {
	data, err := os.ReadFile(os.Getenv("HOME") + "/.nexus-hydra-profile")
	if err != nil {
		return "unknown"
	}
	return strings.TrimSpace(string(data))
}

func detectHydraTier() string {
	out, err := exec.Command("free", "-m").Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "Mem:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			var total int
			fmt.Sscanf(fields[1], "%d", &total)
			switch {
			case total >= 8000:
				return "broadcast"
			case total >= 4000:
				return "hub"
			case total >= 2000:
				return "vanguard"
			default:
				return "phantom"
			}
		}
	}
	return "unknown"
}

// ── Render ───────────────────────────────────────────────────────────────────

func renderHydraSidebar(h HydraStatus, height int) string {
	accent := ActiveTheme.Primary
	muted  := ActiveTheme.Muted

	title := lipgloss.NewStyle().Bold(true).Foreground(accent).Render(" HYDRA ")
	sep   := lipgloss.NewStyle().Foreground(muted).Render(strings.Repeat("─", 18))

	profileColor := lipgloss.Color("#FFB86C")
	if h.GhostGate {
		profileColor = lipgloss.Color("#50FA7B")
	}
	profile := lipgloss.NewStyle().Bold(true).Foreground(profileColor).Render("⚡ " + strings.ToUpper(h.Profile))
	tier    := lipgloss.NewStyle().Foreground(muted).Render("  " + h.Tier + " tier")

	dot := func(up bool, label string) string {
		if up {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("● ") + label
		}
		return lipgloss.NewStyle().Foreground(muted).Render("○ ") + label
	}

	gate := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("🛡 Ghost: OFF")
	if h.GhostGate {
		gate = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("🛡 Ghost: ON")
	}
	ollama := lipgloss.NewStyle().Foreground(lipgloss.Color("#FF5555")).Render("🤖 Ollama: OFF")
	if h.Ports["ollama"] {
		ollama = lipgloss.NewStyle().Foreground(lipgloss.Color("#50FA7B")).Render("🤖 Ollama: ON")
	}

	count    := lipgloss.NewStyle().Foreground(muted).Render(fmt.Sprintf("  %d containers", len(h.Containers)))
	lastPoll := lipgloss.NewStyle().Foreground(muted).Italic(true).Render("  " + h.LastPoll.Format("15:04:05"))

	lines := []string{
		title, sep,
		profile, tier, sep,
		dot(h.Ports["tor"],     "Tor  :9050"),
		dot(h.Ports["i2p"],     "I2P  :4444"),
		dot(h.Ports["haproxy"], "HAProxy"),
		dot(h.Ports["privoxy"], "Privoxy"),
		sep,
		gate, ollama, count, lastPoll,
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1).
		Render(strings.Join(lines, "\n"))
}

func renderHydraTab(h HydraStatus, w int, selected int) string {
	var sb strings.Builder
	accent := ActiveTheme.Primary
	muted  := ActiveTheme.Muted
	green  := lipgloss.Color("#50FA7B")
	red    := lipgloss.Color("#FF5555")
	amber  := lipgloss.Color("#FFB86C")

	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accent).
		Render(" 🐍  HYDRA — Darknet Network Stack ") + "\n\n")

	profileLabel := lipgloss.NewStyle().Bold(true).Foreground(amber).Render(strings.ToUpper(h.Profile))
	tierLabel    := lipgloss.NewStyle().Foreground(muted).Render(h.Tier + " tier")
	gateLabel    := lipgloss.NewStyle().Foreground(red).Render("● GATE OFF")
	if h.GhostGate {
		gateLabel = lipgloss.NewStyle().Foreground(green).Render("● GATE ACTIVE")
	}
	sb.WriteString(fmt.Sprintf(" Profile: %s   Tier: %s   Ghost Gate: %s\n\n",
		profileLabel, tierLabel, gateLabel))

	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render(" ── Ports ──────────────────────────\n"))
	ports := []struct{ key, label string }{
		{"tor",     "Tor SOCKS   :9050"},
		{"i2p",     "I2P HTTP    :4444"},
		{"haproxy", "HAProxy     :8080"},
		{"privoxy", "Privoxy     :8118"},
		{"ollama",  "Ollama AI   :11434"},
	}
	for _, p := range ports {
		if h.Ports[p.key] {
			sb.WriteString(lipgloss.NewStyle().Foreground(green).Render("  ● ") + p.label + "\n")
		} else {
			sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  ○ ") + p.label + "\n")
		}
	}
	sb.WriteString("\n")

	// Unified service list — navigable with cursor
	items := h.AllItems()

	renderSection := func(title string) {
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render(" ── " + title + " " +
			strings.Repeat("─", max(0, 35-len(title))) + "\n"))
	}

	renderItem := func(idx int, item HydraItem) {
		isSelected := idx == selected

		var icon string
		switch item.Kind {
		case "ghost":
			if item.Status == "Active" {
				icon = lipgloss.NewStyle().Foreground(green).Render("  🛡 ")
			} else {
				icon = lipgloss.NewStyle().Foreground(red).Render("  🛡 ")
			}
		case "ollama":
			if item.Status == "Running" {
				icon = lipgloss.NewStyle().Foreground(green).Render("  🤖 ")
			} else {
				icon = lipgloss.NewStyle().Foreground(red).Render("  🤖 ")
			}
		default:
			if strings.Contains(strings.ToLower(item.Status), "exit") {
				icon = lipgloss.NewStyle().Foreground(red).Render("  ■ ")
			} else {
				icon = lipgloss.NewStyle().Foreground(green).Render("  ▶ ")
			}
		}

		cursor := "  "
		if isSelected {
			cursor = lipgloss.NewStyle().Foreground(amber).Bold(true).Render("▸ ")
		}

		statusStr := lipgloss.NewStyle().Foreground(muted).Render(item.Status)
		row := fmt.Sprintf("%s%s%-28s %s", cursor, icon, item.Name, statusStr)
		if isSelected {
			row = lipgloss.NewStyle().Background(lipgloss.Color("#2a2a3a")).Render(row)
		}
		sb.WriteString(row + "\n")
	}

	// System services section
	renderSection("System Services")
	for i, item := range items {
		if item.Kind == "ghost" || item.Kind == "ollama" {
			renderItem(i, item)
		}
	}
	sb.WriteString("\n")

	// Containers section
	renderSection("Containers")
	hasContainers := false
	for i, item := range items {
		if item.Kind == "container" {
			renderItem(i, item)
			hasContainers = true
		}
	}
	if !hasContainers {
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  No Hydra containers running\n"))
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Italic(true).
		Render(fmt.Sprintf(" Last poll: %s   polls every 5s", h.LastPoll.Format("15:04:05"))))

	return sb.String()
}

// HydraTickMsg and HydraHeartbeat live in deck.go
type HydraTickMsg struct{ Status HydraStatus }

func HydraHeartbeat() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return HydraTickMsg{Status: pollHydraStatus()}
	})
}
