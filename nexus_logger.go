package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	LogLevelInfo  = "INFO"
	LogLevelWarn  = "WARN"
	LogLevelError = "ERROR"
)

var (
	nexusLogMu    sync.Mutex
	nexusLogFile  *os.File
	nexusLogPath  string
	nexusLogLevel = LogLevelInfo // default: log INFO and above
)

// logLevelRank maps level strings to numeric ranks for comparison.
var logLevelRank = map[string]int{
	"DEBUG": 0,
	"INFO":  1,
	"WARN":  2,
	"ERROR": 3,
}

// SetLogLevel sets the minimum log level from a string (debug/info/warn/error).
func SetLogLevel(level string) {
	upper := strings.ToUpper(level)
	if _, ok := logLevelRank[upper]; ok {
		nexusLogMu.Lock()
		nexusLogLevel = upper
		nexusLogMu.Unlock()
	}
}

// InitNexusLogger opens (or creates) the log file at ~/.nexus/logs/nexus.log.
func InitNexusLogger() {
	nexusLogMu.Lock()
	defer nexusLogMu.Unlock()

	if nexusLogFile != nil {
		return
	}

	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	logDir := filepath.Join(home, ".nexus", "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return
	}

	nexusLogPath = filepath.Join(logDir, "nexus.log")
	f, err := os.OpenFile(nexusLogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	nexusLogFile = f
	fmt.Fprintf(nexusLogFile, "\n=== NeXuS session started %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
}

// NexusLog writes a structured log entry directly to the log file.
// Entries below the current log level are silently dropped.
func NexusLog(level, component, msg string) {
	nexusLogMu.Lock()
	minRank := logLevelRank[nexusLogLevel]
	thisRank, ok := logLevelRank[strings.ToUpper(level)]
	f := nexusLogFile
	nexusLogMu.Unlock()

	if !ok || thisRank < minRank || f == nil {
		return
	}

	ts := time.Now().Format("2006-01-02 15:04:05")
	entry := fmt.Sprintf("[%s] %-5s %-12s %s\n", ts, level, component, msg)

	nexusLogMu.Lock()
	defer nexusLogMu.Unlock()
	f.WriteString(entry)
}

// GetRecentLogs reads the last n lines from the log file (all levels).
func GetRecentLogs(n int) []string {
	nexusLogMu.Lock()
	path := nexusLogPath
	nexusLogMu.Unlock()

	if path == "" || n <= 0 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) == 0 {
		return nil
	}
	if n > len(lines) {
		n = len(lines)
	}
	return lines[len(lines)-n:]
}

// GetRecentErrors reads the last n WARN/ERROR lines directly from the log file.
func GetRecentErrors(n int) []string {
	nexusLogMu.Lock()
	path := nexusLogPath
	nexusLogMu.Unlock()

	if path == "" || n <= 0 {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Collect all WARN/ERROR lines
	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) > 20 && (contains(line, " ERROR ") || contains(line, " WARN  ")) {
			lines = append(lines, line)
		}
	}

	if len(lines) == 0 {
		return nil
	}
	if n > len(lines) {
		n = len(lines)
	}
	return lines[len(lines)-n:]
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// CloseNexusLogger flushes and closes the log file.
func CloseNexusLogger() {
	nexusLogMu.Lock()
	defer nexusLogMu.Unlock()
	if nexusLogFile != nil {
		fmt.Fprintf(nexusLogFile, "=== NeXuS session ended %s ===\n", time.Now().Format("2006-01-02 15:04:05"))
		nexusLogFile.Close()
		nexusLogFile = nil
	}
}
