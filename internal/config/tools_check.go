package config

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"
)

// ToolStatus represents the availability status of a tool
type ToolStatus struct {
	Available bool
	Message   string
	// Detailed error information (only set when Available is false)
	ExitCode int
	Stdout   string
	Stderr   string
}

// CheckAvailability runs the tool's check command to verify it's available
func (t *ExternalTool) CheckAvailability() ToolStatus {
	if t.Check.Command == "" {
		// No check defined, assume available if access command exists
		if t.Access.Type == "shell" && t.Access.Command != "" {
			return t.checkCommandExists(t.Access.Command)
		}
		return ToolStatus{Available: true, Message: "no check defined"}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", t.Check.Command)

	// Set environment variables from tool config
	if env := t.BuildEnv(); env != nil {
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	stdoutStr := strings.TrimSpace(stdout.String())
	stderrStr := strings.TrimSpace(stderr.String())

	if err != nil {
		// Extract exit code if available
		exitCode := -1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}

		return ToolStatus{
			Available: false,
			Message:   "check failed: " + err.Error(),
			ExitCode:  exitCode,
			Stdout:    stdoutStr,
			Stderr:    stderrStr,
		}
	}

	// If expected string is set, verify it's in the output
	output := stdoutStr + stderrStr
	if t.Check.Expected != "" {
		if !strings.Contains(output, t.Check.Expected) {
			return ToolStatus{
				Available: false,
				Message:   "expected output not found: " + t.Check.Expected,
				ExitCode:  0,
				Stdout:    stdoutStr,
				Stderr:    stderrStr,
			}
		}
	}

	return ToolStatus{
		Available: true,
		Message:   "check passed",
	}
}

// checkCommandExists checks if a command exists in PATH
func (t *ExternalTool) checkCommandExists(command string) ToolStatus {
	// Extract base command (first word)
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return ToolStatus{Available: false, Message: "empty command"}
	}

	_, err := exec.LookPath(parts[0])
	if err != nil {
		return ToolStatus{
			Available: false,
			Message:   "command not found in PATH",
		}
	}

	return ToolStatus{
		Available: true,
		Message:   "command found",
	}
}

// LoadAndCheckTools loads external tools and checks their availability
func LoadAndCheckTools() ([]*ExternalTool, map[string]ToolStatus, error) {
	tools, err := LoadExternalTools()
	if err != nil {
		return nil, nil, err
	}

	statuses := make(map[string]ToolStatus)
	var availableTools []*ExternalTool

	for _, tool := range tools {
		status := tool.CheckAvailability()
		statuses[tool.Name] = status
		if status.Available {
			availableTools = append(availableTools, tool)
		}
	}

	return availableTools, statuses, nil
}
