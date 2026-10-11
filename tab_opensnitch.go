package main

// tab_opensnitch.go — OpenSnitch tab render layer
// Shows live connection events, allows accept/deny per event.
// Completely self-contained — only reads from DeckModel.snitch* fields.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m DeckModel) renderSnitchTab() string {
	if m.snitchRulesView {
		return m.renderSnitchRules()
	}
	return m.renderSnitchEvents()
}

func (m DeckModel) renderSnitchEvents() string {
	var sb strings.Builder
	accent := ActiveTheme.Primary
	muted  := ActiveTheme.Muted
	green  := lipgloss.Color("#50FA7B")
	red    := lipgloss.Color("#FF5555")
	amber  := lipgloss.Color("#FFB86C")

	// ── Header ──────────────────────────────────────────────
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accent).
		Render(" 👁  OPENSNITCH — App Connection Monitor ") + "\n\n")

	// ── Daemon status ────────────────────────────────────────
	if m.snitchActive {
		sb.WriteString(lipgloss.NewStyle().Foreground(green).Render("  ● opensnitchd RUNNING") + "\n\n")
	} else {
		sb.WriteString(lipgloss.NewStyle().Foreground(red).Render("  ○ opensnitchd STOPPED") + "  " +
			lipgloss.NewStyle().Foreground(muted).Render("start: ~/claude/nexus-network-stack/scripts/nexus-opensnitch.sh start") + "\n\n")
	}

	// ── Column headers ───────────────────────────────────────
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(muted)
	sb.WriteString(headerStyle.Render(fmt.Sprintf(
		"  %-6s %-22s %-22s %-8s %-10s",
		"", "PROCESS", "DESTINATION", "PORT", "STATUS",
	)) + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  " + strings.Repeat("─", 72)) + "\n")

	// ── Event list (with sliding window scrolling) ─────────────────────────
	if len(m.snitchEvents) == 0 {
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  No connection events yet."))
		if !m.snitchActive {
			sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render(" Start opensnitchd to begin monitoring."))
		}
		sb.WriteString("\n")
	} else {
		// Calculate viewport: header(7) + footer(6) + spacing = ~15 lines overhead
		maxVisible := m.height - 18
		if maxVisible < 5 { maxVisible = 5 }
		
		start := 0
		if m.snitchSelected >= maxVisible {
			start = m.snitchSelected - maxVisible + 1
		}
		end := start + maxVisible
		if end > len(m.snitchEvents) { end = len(m.snitchEvents) }

		if start > 0 {
			sb.WriteString(lipgloss.NewStyle().Foreground(amber).Render("  ↑ (more events above)") + "\n")
		}

		for i := start; i < end; i++ {
			e := m.snitchEvents[i]
			isSelected := i == m.snitchSelected

			// Status color
			var statusStyle lipgloss.Style
			switch e.Action {
			case "allowed":
				statusStyle = lipgloss.NewStyle().Foreground(green)
			case "denied":
				statusStyle = lipgloss.NewStyle().Foreground(red)
			default:
				statusStyle = lipgloss.NewStyle().Foreground(amber)
			}

			cursor := "  "
			if isSelected {
				cursor = lipgloss.NewStyle().Foreground(amber).Bold(true).Render("▸ ")
			}

			proc := truncate(e.Process, 22)
			dst  := truncate(e.Dst, 22)
			port := truncate(e.Port, 8)

			row := fmt.Sprintf("%s%-22s %-22s %-8s %s",
				cursor,
				proc,
				dst,
				port,
				statusStyle.Render(e.Action),
			)

			if isSelected {
				row = lipgloss.NewStyle().Background(lipgloss.Color("#2a2a3a")).Render(row)
			}
			sb.WriteString("  " + row + "\n")
		}

		if end < len(m.snitchEvents) {
			sb.WriteString(lipgloss.NewStyle().Foreground(amber).Render("  ↓ (more events below)") + "\n")
		}
	}

	// ── Selected event detail ────────────────────────────────
	if len(m.snitchEvents) > 0 && m.snitchSelected < len(m.snitchEvents) {
		e := m.snitchEvents[m.snitchSelected]
		sb.WriteString("\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  " + strings.Repeat("─", 72)) + "\n")
		sb.WriteString(lipgloss.NewStyle().Foreground(accent).Render("  Selected: "))
		sb.WriteString(fmt.Sprintf("process=%s  dst=%s  port=%s  status=%s\n",
			e.Process, e.Dst, e.Port, e.Action))
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render(
			"  [a] Allow this process always  [d] Deny this process always  [↑/↓] Navigate\n"))
	}

	// ── Rules dir hint ───────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Italic(true).
		Render(fmt.Sprintf("  Rules: %s   [r] View rules sheet", rulesDir())))

	return sb.String()
}

func (m DeckModel) renderSnitchRules() string {
	var sb strings.Builder
	accent := ActiveTheme.Primary
	muted  := ActiveTheme.Muted
	green  := lipgloss.Color("#50FA7B")
	red    := lipgloss.Color("#FF5555")

	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(accent).
		Render(" 📋  OPENSNITCH — Rules Sheet ") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).
		Render("  [r] Back to live feed\n\n"))

	// Load rules from disk
	rules := loadSnitchRules()

	var allowed, denied []SnitchRule
	for _, r := range rules {
		if r.Action == "allow" {
			allowed = append(allowed, r)
		} else {
			denied = append(denied, r)
		}
	}

	// ── Whitelist ────────────────────────────────────────────
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(green).
		Render(" ✅  WHITELIST — Always Allow") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  " + strings.Repeat("─", 60)) + "\n")
	if len(allowed) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  (none — press [a] on live feed to add)\n"))
	} else {
		for _, r := range allowed {
			sb.WriteString(lipgloss.NewStyle().Foreground(green).Render("  ✓ "))
			sb.WriteString(fmt.Sprintf("%-20s  %s: %s\n",
				truncate(r.Name, 20),
				r.Operator.Operand,
				r.Operator.Data,
			))
		}
	}

	sb.WriteString("\n")

	// ── Blacklist ────────────────────────────────────────────
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(red).
		Render(" 🚫  BLACKLIST — Always Deny") + "\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  " + strings.Repeat("─", 60)) + "\n")
	if len(denied) == 0 {
		sb.WriteString(lipgloss.NewStyle().Foreground(muted).Render("  (none — press [d] on live feed to add)\n"))
	} else {
		for _, r := range denied {
			sb.WriteString(lipgloss.NewStyle().Foreground(red).Render("  ✗ "))
			sb.WriteString(fmt.Sprintf("%-20s  %s: %s\n",
				truncate(r.Name, 20),
				r.Operator.Operand,
				r.Operator.Data,
			))
		}
	}

	sb.WriteString("\n")
	sb.WriteString(lipgloss.NewStyle().Foreground(muted).Italic(true).
		Render(fmt.Sprintf("  %d rules total (%d allow, %d deny)   Rules dir: %s",
			len(rules), len(allowed), len(denied), rulesDir())))

	return sb.String()
}

func loadSnitchRules() []SnitchRule {
	dir := rulesDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var rules []SnitchRule
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var r SnitchRule
		if json.Unmarshal(data, &r) == nil {
			rules = append(rules, r)
		}
	}
	return rules
}

func truncate(s string, n int) string {
	if len(s) == 0 {
		return "-"
	}
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
