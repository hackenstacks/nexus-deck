package ghostgate

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// AuditEntry mirrors the orchestrator's ~/.nexus/audit.log JSONL format
type AuditEntry struct {
	Timestamp    float64 `json:"timestamp"`
	AIFingerprint string `json:"ai_fingerprint"`
	AIKeyHash    string  `json:"ai_key_hash"`
	ActionProposed string `json:"action_proposed"`
	UserApproved bool    `json:"user_approved"`
	Executed     bool    `json:"executed"`
	PrevHash     string  `json:"prev_hash"`
	EntryHash    string  `json:"entry_hash"`
}

func (e AuditEntry) Time() time.Time {
	sec := int64(e.Timestamp)
	nsec := int64((e.Timestamp - float64(sec)) * 1e9)
	return time.Unix(sec, nsec)
}

// ShortFingerprint returns first 8 chars of fingerprint for display
func (e AuditEntry) ShortFingerprint() string {
	if len(e.AIFingerprint) >= 8 {
		return e.AIFingerprint[:8]
	}
	return e.AIFingerprint
}

// StatusIcon returns visual indicator for approved/denied
func (e AuditEntry) StatusIcon() string {
	if e.UserApproved && e.Executed {
		return "✓"
	}
	if !e.UserApproved {
		return "✗"
	}
	return "~"
}

// ShortAction returns a single-line summary of the action
func (e AuditEntry) ShortAction() string {
	action := e.ActionProposed
	// Trim newlines and excess whitespace
	for i, c := range action {
		if c == '\n' || c == '\r' {
			action = action[:i]
			break
		}
	}
	if len(action) > 48 {
		return action[:45] + "..."
	}
	return action
}

func auditLogPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".nexus", "audit.log")
}

// ReadRecentEntries reads the last n entries from the audit log.
// Returns entries in reverse-chronological order (newest first).
func ReadRecentEntries(n int) ([]AuditEntry, error) {
	path := auditLogPath()
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("audit log not found: %s (start orchestrator first)", path)
	}
	defer f.Close()

	var all []AuditEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip malformed lines
		}
		// Skip genesis entry
		if entry.ActionProposed == "GENESIS" {
			continue
		}
		all = append(all, entry)
	}

	// Return last n in reverse order (newest first)
	if len(all) > n {
		all = all[len(all)-n:]
	}
	// Reverse
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	return all, nil
}

// AuditStats returns counts of approved vs denied entries
func AuditStats() (total, approved, denied int, err error) {
	path := auditLogPath()
	f, err := os.Open(path)
	if err != nil {
		return 0, 0, 0, err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry AuditEntry
		if json.Unmarshal(line, &entry) != nil {
			continue
		}
		if entry.ActionProposed == "GENESIS" {
			continue
		}
		total++
		if entry.UserApproved {
			approved++
		} else {
			denied++
		}
	}
	return total, approved, denied, nil
}
