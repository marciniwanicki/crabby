package tools

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/marciniwanicki/crabby/internal/config"
)

const shellTimeout = 30 * time.Second

// ShellTool executes shell commands from an allowlist
type ShellTool struct {
	settings *config.Settings
}

// NewShellTool creates a new shell tool
func NewShellTool(settings *config.Settings) *ShellTool {
	return &ShellTool{
		settings: settings,
	}
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	return "Execute a shell command. Only commands from the allowlist are permitted: " +
		strings.Join(t.settings.Tools.Shell.Allowlist, ", ")
}

func (t *ShellTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The shell command to execute",
			},
		},
		"required": []string{"command"},
	}
}

func (t *ShellTool) Execute(args map[string]any) (string, error) {
	commandRaw, ok := args["command"]
	if !ok {
		return "", fmt.Errorf("missing required parameter: command")
	}

	command, ok := commandRaw.(string)
	if !ok {
		return "", fmt.Errorf("command must be a string")
	}

	// Validate command against allowlist
	if err := t.validateCommand(command); err != nil {
		return "", err
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	// Combine output
	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("command timed out after %v", shellTimeout)
	}

	if err != nil {
		return output, fmt.Errorf("command failed: %w", err)
	}

	return output, nil
}

func (t *ShellTool) validateCommand(command string) error {
	// Check for shell operators that could be used to chain commands
	dangerousPatterns := []string{"&&", "||", ";", "|", "`", "$(", "${", ">", "<"}
	for _, pattern := range dangerousPatterns {
		if strings.Contains(command, pattern) {
			return fmt.Errorf("command contains disallowed pattern: %s", pattern)
		}
	}

	// Extract the base command (first word)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}

	baseCmd := parts[0]

	// Check if base command is in allowlist
	if !t.settings.IsCommandAllowed(baseCmd) {
		return fmt.Errorf("command not in allowlist: %s (allowed: %s)",
			baseCmd, strings.Join(t.settings.Tools.Shell.Allowlist, ", "))
	}

	return nil
}
