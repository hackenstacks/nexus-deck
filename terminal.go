package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const workspacesRoot = ".nexus/workspaces"

// WorkspaceDir returns the path to a character's workspace directory.
// Creates it if it doesn't exist.
func WorkspaceDir(charID string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, workspacesRoot, charID)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("failed to create workspace: %w", err)
	}
	return dir, nil
}

// canExecute returns true if the command is on the safe whitelist.
// Anything not on this list is silently denied.
func canExecute(cmd string) bool {
	parts := strings.Fields(cmd)
	if len(parts) == 0 {
		return false
	}
	allowed := map[string]bool{
		"ls": true, "cat": true, "echo": true, "pwd": true,
		"mkdir": true, "touch": true, "cp": true, "mv": true,
		"grep": true, "find": true, "wc": true, "head": true,
		"tail": true, "date": true, "whoami": true, "uname": true,
		"python3": true, "python": true, "node": true,
		"go": true, "bash": true, "sh": true,
	}
	return allowed[parts[0]]
}

// ExecResult holds the output of a sandboxed command execution.
type ExecResult struct {
	Command  string
	Output   string
	ExitCode int
	Duration time.Duration
	Error    error
}

// RunInWorkspace executes a command inside a character's workspace directory.
// Output is capped at 4000 chars. Timeout: 30 seconds.
func RunInWorkspace(charID, command string) ExecResult {
	start := time.Now()

	if !canExecute(command) {
		return ExecResult{
			Command:  command,
			Output:   fmt.Sprintf("⛔ Command not permitted: %s", strings.Fields(command)[0]),
			ExitCode: 1,
			Duration: time.Since(start),
		}
	}

	dir, err := WorkspaceDir(charID)
	if err != nil {
		return ExecResult{Command: command, Error: err, ExitCode: 1}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	parts := strings.Fields(command)
	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	
	exitCode := 0
	if cmd.ProcessState != nil {
		exitCode = cmd.ProcessState.ExitCode()
	} else if err != nil {
		exitCode = 1
	}

	result := ExecResult{
		Command:  command,
		ExitCode: exitCode,
		Duration: time.Since(start),
		Error:    err,
	}

	output := string(out)
	if len(output) > 4000 {
		output = output[:4000] + "\n[...output capped at 4000 chars]"
	}
	result.Output = output
	return result
}
