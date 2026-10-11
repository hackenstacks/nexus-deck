package main

// gate.go — Ghost Gate HUD tab for the MNEMOSYNE Deck.
// Reads real audit data from ~/.nexus/audit.log via the nexus-darknet ghostgate package.
// Falls back to network health from GateStats (Command Center API) when audit log unavailable.

import (
	"fmt"
	"strings"
	"time"

	"char-gen-cli/ghostgate"

	"github.com/charmbracelet/lipgloss"
)

// GateState holds Ghost Gate UI state (command input) for the Deck.
type GateState struct {
	cmdInput      string
	cmdFocused    bool
	cmdResult     string
	cmdResultTime time.Time
}

// renderGateHUD renders the full Ghost Gate tab.
// Uses live audit log data from the Ghost Gate orchestrator.
// Falls back to GateStats from the Command Center API when audit log is unavailable.
func renderGateHUD(gs GateState, gate GateStats, w int) string {
	var sb strings.Builder

	// ── Header ─────────────────────────────────────────────────
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).
		Render(" 👻  GHOST GATE — Zero Trust Egress Controller ") + "\n\n")

	// ── Gate active status ──────────────────────────────────────
	if ghostgate.IsActive() {
		sb.WriteString(SuccessStyle().Render("  ● GATE ACTIVE") + "\n\n")
	} else {
		sb.WriteString(WarningStyle().Render("  ○ GATE INACTIVE") +
			MutedStyle().Render("  — start: python nexus.py") + "\n\n")
	}

	// ── Audit stats ─────────────────────────────────────────────
	total, approved, denied, statsErr := ghostgate.AuditStats()
	if statsErr == nil {
		sb.WriteString(fmt.Sprintf("  Total: %d    %s    %s\n\n",
			total,
			lipgloss.NewStyle().Foreground(ActiveTheme.Success).
				Render(fmt.Sprintf("✓ Approved: %d", approved)),
			lipgloss.NewStyle().Foreground(ActiveTheme.Error).
				Render(fmt.Sprintf("✗ Denied: %d", denied)),
		))
	}

	// ── Recent audit log entries ────────────────────────────────
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).
		Render(" RECENT ACTIVITY") + "\n")
	sb.WriteString(gateSeparator(w) + "\n")

	entries, readErr := ghostgate.ReadRecentEntries(10)
	if readErr != nil {
		sb.WriteString(ErrorStyle().Render(fmt.Sprintf("  %s", readErr.Error())) + "\n")
		sb.WriteString(MutedStyle().Render("  → start orchestrator: python nexus.py  (~/claude/nexus-orchestrator/)") + "\n\n")
		// Show network tunnel status from Command Center API as fallback
		sb.WriteString(renderTunnelStatus(gate, w))
	} else if len(entries) == 0 {
		sb.WriteString(MutedStyle().Render("  No events recorded yet.\n"))
		sb.WriteString(renderTunnelStatus(gate, w))
	} else {
		sb.WriteString(MutedStyle().Render(fmt.Sprintf(
			"  %-8s  %-10s  %s\n", "TIME", "IDENTITY", "ACTION")))
		for _, e := range entries {
			icon := e.StatusIcon()
			ts := e.Time().Format("15:04:05")
			fp := e.ShortFingerprint()
			action := e.ShortAction()
			iconStyle := SuccessStyle()
			actionRender := action
			if !e.UserApproved {
				iconStyle = ErrorStyle()
				actionRender = ErrorStyle().Render(action)
			}
			sb.WriteString(fmt.Sprintf("  %s  %-8s  %-10s  %s\n",
				iconStyle.Render(icon), ts, fp, actionRender))
		}
		sb.WriteString("\n")
		sb.WriteString(renderTunnelStatus(gate, w))
	}

	// ── Command input ────────────────────────────────────────────
	sb.WriteString("\n" + MutedStyle().Render(" GATE COMMANDS") + "\n")
	prompt := "> " + gs.cmdInput
	if gs.cmdFocused {
		prompt = lipgloss.NewStyle().Foreground(ActiveTheme.Primary).Render("> ") +
			gs.cmdInput + "█"
	}
	sb.WriteString(lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ActiveTheme.Primary).
		Padding(0, 1).Width(w-10).
		Render(prompt) + "\n")

	if gs.cmdResult != "" && time.Since(gs.cmdResultTime) < 10*time.Second {
		sb.WriteString(lipgloss.NewStyle().
			Foreground(ActiveTheme.Success).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ActiveTheme.Success).
			Padding(0, 1).Width(w-10).
			Render(gs.cmdResult) + "\n")
	}

	sb.WriteString("\n" + MutedStyle().Render(
		" [ctrl+i] focus  [Enter] exec  [Esc] unfocus  [ctrl+r] refresh"+
			"  ·  status  report  issue <dst> <proto>  revoke <id>"))

	return sb.String()
}

// renderTunnelStatus renders the darknet tunnel block from GateStats.
func renderTunnelStatus(gate GateStats, w int) string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Bold(true).Foreground(ActiveTheme.Primary).
		Render(" DARKNET TUNNELS") + "\n")
	sb.WriteString(gateSeparator(w) + "\n")

	tunnels := []struct{ name, speed string }{
		{"Tor", gate.TorSpeed},
		{"I2P", gate.I2PSpeed},
		{"Yggdrasil", gate.YggdrasilState},
	}
	for _, t := range tunnels {
		icon := SuccessStyle().Render("●")
		if t.speed == "" || t.speed == "down" || t.speed == "stopped" {
			icon = ErrorStyle().Render("●")
		}
		sb.WriteString(fmt.Sprintf("  %s  %-12s  %s\n", icon, t.name,
			MutedStyle().Render(t.speed)))
	}
	if len(gate.RecentPermits) > 0 {
		sb.WriteString("\n" + MutedStyle().Render(" RECENT PERMITS\n"))
		for _, p := range gate.RecentPermits {
			sb.WriteString("  " + p + "\n")
		}
	}
	if gate.Intercepted > 0 {
		sb.WriteString(WarningStyle().Render(
			fmt.Sprintf("\n  ⚠  INTERCEPTED: %d", gate.Intercepted)) + "\n")
	}
	return sb.String()
}

func gateSeparator(w int) string {
	n := w - 4
	if n < 10 {
		n = 40
	}
	return MutedStyle().Render(strings.Repeat("─", n))
}
