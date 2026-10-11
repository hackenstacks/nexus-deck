package ghostgate

import (
	"os"
	"os/exec"
	"strings"
)

func IsActive() bool {
	// 1. New Priority Check: Sovereign State Export from Orchestrator
	// This file is updated by the Python ghost gate manager
	home, _ := os.UserHomeDir()
	statePath := home + "/.nexus/gate_state.json"
	if _, err := os.Stat(statePath); err == nil {
		// Optimization: We could parse the JSON here if we needed specific permit counts
		return true
	}

	// 2. Real Kernel nftables (Requires root)
	cmd := exec.Command("nft", "list", "tables", "inet")
	out, err := cmd.CombinedOutput()
	if err == nil && strings.Contains(string(out), "nexus_ghost") {
		return true
	}

	// 3. Fallback: Simulation Lockfile
	if _, err := os.Stat("/tmp/nexus_ghost.lock"); err == nil {
		return true
	}
	
	return false
}

func GetStatusString() string {
	if IsActive() {
		return "ACTIVE"
	}
	return "INACTIVE"
}
