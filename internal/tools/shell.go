package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/marciniwanicki/crabby/internal/config"
)

const shellTimeout = 30 * time.Second
const discoveryTimeout = 60 * time.Second

// LLMClient interface for making LLM calls during tool discovery
type LLMClient interface {
	// SimpleChat makes a simple chat completion call without tools
	SimpleChat(ctx context.Context, systemPrompt, userMessage string) (string, error)
}

// CommandObserver is called when a shell command is executed
type CommandObserver func(command string, isDiscovery bool)

// Well-known system commands that don't need help discovery
var wellKnownCommands = map[string]bool{
	"ls": true, "cat": true, "head": true, "tail": true, "grep": true,
	"find": true, "wc": true, "sort": true, "uniq": true, "cut": true,
	"echo": true, "printf": true, "date": true, "cal": true,
	"pwd": true, "cd": true, "mkdir": true, "rmdir": true, "rm": true,
	"cp": true, "mv": true, "touch": true, "chmod": true, "chown": true,
	"whoami": true, "id": true, "groups": true, "uname": true,
	"hostname": true, "uptime": true, "ps": true, "top": true, "kill": true,
	"df": true, "du": true, "free": true, "mount": true, "umount": true,
	"ping": true, "curl": true, "wget": true, "ssh": true, "scp": true,
	"tar": true, "zip": true, "unzip": true, "gzip": true, "gunzip": true,
	"sed": true, "awk": true, "tr": true, "diff": true, "patch": true,
	"man": true, "which": true, "whereis": true, "type": true,
	"env": true, "export": true, "set": true, "unset": true,
	"true": true, "false": true, "test": true, "sleep": true,
	"xargs": true, "tee": true, "less": true, "more": true,
}

// ShellTool executes shell commands from an allowlist
type ShellTool struct {
	settings      *config.Settings
	externalTools []*config.ExternalTool
	llm           LLMClient
	userRequest   string          // The original user request for context during discovery
	observer      CommandObserver // Optional callback when commands are executed
}

// NewShellTool creates a new shell tool
func NewShellTool(settings *config.Settings) *ShellTool {
	return &ShellTool{
		settings: settings,
	}
}

// NewShellToolWithExternalTools creates a shell tool with external tool definitions
func NewShellToolWithExternalTools(settings *config.Settings, externalTools []*config.ExternalTool) *ShellTool {
	return &ShellTool{
		settings:      settings,
		externalTools: externalTools,
	}
}

// NewShellToolWithLLM creates a shell tool with LLM support for smart discovery
func NewShellToolWithLLM(settings *config.Settings, externalTools []*config.ExternalTool, llm LLMClient) *ShellTool {
	return &ShellTool{
		settings:      settings,
		externalTools: externalTools,
		llm:           llm,
	}
}

// SetUserRequest sets the current user request for context during discovery
func (t *ShellTool) SetUserRequest(request string) {
	t.userRequest = request
}

// SetCommandObserver sets a callback that's invoked when any shell command is executed
func (t *ShellTool) SetCommandObserver(observer CommandObserver) {
	t.observer = observer
}

func (t *ShellTool) Name() string {
	return "shell"
}

func (t *ShellTool) Description() string {
	desc := "Execute a shell command. Only commands from the allowlist are permitted: " +
		strings.Join(t.settings.Tools.Shell.Allowlist, ", ")

	// Add external tools
	if len(t.externalTools) > 0 {
		var extNames []string
		for _, ext := range t.externalTools {
			if ext.Access.Type == "shell" {
				extNames = append(extNames, ext.Access.Command)
			}
		}
		if len(extNames) > 0 {
			desc += ", " + strings.Join(extNames, ", ")
		}
	}

	return desc
}

// GetExternalToolsPrompt returns a formatted description of all external tools for the system prompt
func (t *ShellTool) GetExternalToolsPrompt() string {
	if len(t.externalTools) == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n## Available External Tools\n\n")
	sb.WriteString("The following specialized tools are available via the shell. ")
	sb.WriteString("When you first use any of these tools, the system will automatically discover ")
	sb.WriteString("available subcommands and options by calling --help. Use this discovered information ")
	sb.WriteString("to construct correct commands.\n\n")

	for _, ext := range t.externalTools {
		sb.WriteString(fmt.Sprintf("- **%s**: %s", ext.Access.Command, ext.Description))
		if ext.WhenToUse != "" {
			sb.WriteString(fmt.Sprintf(" (%s)", ext.WhenToUse))
		}
		sb.WriteString("\n")
	}

	return sb.String()
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

	// Check if this is an external tool that needs discovery
	// If discovery happens, we return ONLY the discovery info and don't execute the command
	// This gives the agent a chance to learn the tool before using it
	discoveryInfo, isFirstUse := t.runToolDiscoveryIfNeeded(command)
	if isFirstUse && discoveryInfo != "" {
		return discoveryInfo + "\n\n" +
			"=== IMPORTANT: Tool discovery complete. Do NOT re-run the same command. ===\n" +
			"Use the discovered information above to construct a VALID command.\n" +
			"Check the available subcommands and their options before proceeding.", nil
	}

	// Notify observer of command execution
	if t.observer != nil {
		t.observer(command, false)
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(context.Background(), shellTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	// Set environment variables if this is an external tool
	if env := t.getExternalToolEnv(command); env != nil {
		cmd.Env = env
	}

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

// getExternalToolEnv returns the environment variables for an external tool command.
// Returns nil if no external tool matches or no env config is set.
func (t *ShellTool) getExternalToolEnv(command string) []string {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}

	baseCmd := parts[0]

	for _, ext := range t.externalTools {
		if ext.Access.Type == "shell" && ext.Access.Command == baseCmd {
			return ext.BuildEnv()
		}
	}

	return nil
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

	// Check if base command is in settings allowlist
	if t.settings.IsCommandAllowed(baseCmd) {
		return nil
	}

	// Check if it's an external tool
	for _, ext := range t.externalTools {
		if ext.Access.Type == "shell" && ext.Access.Command == baseCmd {
			return nil
		}
	}

	return fmt.Errorf("command not in allowlist: %s (allowed: %s)",
		baseCmd, strings.Join(t.settings.Tools.Shell.Allowlist, ", "))
}

// runToolDiscoveryIfNeeded checks if this is an external tool and runs discovery.
// Returns the discovery text and whether this is an external tool.
func (t *ShellTool) runToolDiscoveryIfNeeded(command string) (string, bool) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", false
	}

	baseCmd := parts[0]

	// Skip well-known system commands
	if wellKnownCommands[baseCmd] {
		return "", false
	}

	// Check if this is an external tool - if so, run discovery
	for _, ext := range t.externalTools {
		if ext.Access.Type == "shell" && ext.Access.Command == baseCmd {
			// Always run discovery for external tools (no caching)
			discoveryText := t.runExternalToolDiscovery(ext)
			return discoveryText, true
		}
	}

	return "", false
}

// DiscoveryResponse represents the LLM's response for iterative discovery
type DiscoveryResponse struct {
	Command  string `json:"command"`
	Continue bool   `json:"continue"`
	Error    string `json:"error,omitempty"`
}

// runExternalToolDiscovery runs an iterative discovery loop for an external tool.
// It uses the LLM to guide exploration step-by-step until a complete answer is found.
func (t *ShellTool) runExternalToolDiscovery(tool *config.ExternalTool) string {
	var result strings.Builder
	ctx, cancel := context.WithTimeout(context.Background(), discoveryTimeout)
	defer cancel()

	baseCmd := tool.Access.Command

	result.WriteString(fmt.Sprintf("=== Tool Discovery: %s ===\n\n", baseCmd))
	result.WriteString(fmt.Sprintf("**Description:** %s\n", tool.Description))
	if tool.WhenToUse != "" {
		result.WriteString(fmt.Sprintf("**When to use:** %s\n\n", tool.WhenToUse))
	}

	// If no LLM or no user request, fall back to simple help discovery
	if t.llm == nil || t.userRequest == "" {
		return t.runSimpleDiscovery(tool, &result)
	}

	// Iterative discovery loop
	const maxIterations = 10
	var discoveredHelp []string

discoveryLoop:
	for i := 0; i < maxIterations; i++ {
		select {
		case <-ctx.Done():
			result.WriteString("\n(Discovery timeout reached)\n")
			break discoveryLoop
		default:
		}

		// Ask LLM what command to run next
		nextAction, err := t.askNextDiscoveryStep(ctx, baseCmd, t.userRequest, discoveredHelp)
		if err != nil {
			result.WriteString(fmt.Sprintf("\n## Discovery error: %v\n", err))
			break
		}

		if nextAction.Error != "" {
			result.WriteString(fmt.Sprintf("\n## LLM reported error: %s\n", nextAction.Error))
			break
		}

		if nextAction.Command == "" {
			result.WriteString("\n## Discovery complete (no more commands to run)\n")
			break
		}

		// Validate the command starts with our tool
		if !strings.HasPrefix(nextAction.Command, baseCmd) {
			result.WriteString(fmt.Sprintf("\n## Invalid command (must start with %s): %s\n", baseCmd, nextAction.Command))
			break
		}

		// Execute the discovery command
		result.WriteString(fmt.Sprintf("\n## Step %d: Running `%s`\n", i+1, nextAction.Command))
		output := t.executeDiscoveryCommand(ctx, nextAction.Command, tool)

		if output == "" {
			result.WriteString("(No output)\n")
		} else {
			// Truncate if needed
			if len(output) > 2000 {
				output = output[:2000] + "\n... (truncated)"
			}
			result.WriteString("```\n")
			result.WriteString(output)
			result.WriteString("\n```\n")

			// Track discovered help for context
			discoveredHelp = append(discoveredHelp, fmt.Sprintf("Command: %s\nOutput:\n%s", nextAction.Command, output))
		}

		if !nextAction.Continue {
			result.WriteString("\n## Discovery complete\n")
			break
		}
	}

	result.WriteString("\n=== Use the discovered information above to construct your command. ===\n")

	// Truncate total if needed
	output := result.String()
	if len(output) > 15000 {
		output = output[:15000] + "\n... (discovery output truncated)"
	}

	return output
}

// askNextDiscoveryStep asks the LLM what command to run next in discovery
func (t *ShellTool) askNextDiscoveryStep(ctx context.Context, toolName, userRequest string, previousOutputs []string) (*DiscoveryResponse, error) {
	systemPrompt := fmt.Sprintf(`You are exploring the '%s' CLI tool to help answer a user's question.
Your goal is to discover the exact command(s) needed to fulfill the user's request.

RULES:
1. Start with '%s --help' or '%s help' to see available commands
2. Drill down into relevant subcommands by running their --help
3. Stop when you've found the complete command syntax needed
4. All commands MUST start with '%s'

RESPONSE FORMAT - Reply with ONLY valid JSON, nothing else:
- To run a command: {"command": "%s <subcommand> --help", "continue": true}
- When done discovering: {"command": "%s <final-command>", "continue": false}
- If stuck or error: {"error": "explanation"}

Keep exploring until you find the specific command that answers the user's question.`,
		toolName, toolName, toolName, toolName, toolName, toolName)

	var userMessage strings.Builder
	userMessage.WriteString(fmt.Sprintf("User request: %s\n\n", userRequest))

	if len(previousOutputs) == 0 {
		userMessage.WriteString("This is the first step. Start by getting the main help.\n")
	} else {
		userMessage.WriteString("Previous discovery steps:\n")
		for i, output := range previousOutputs {
			// Limit context size
			if i > 3 {
				userMessage.WriteString(fmt.Sprintf("\n... and %d more previous outputs\n", len(previousOutputs)-i))
				break
			}
			// Truncate individual outputs in context
			o := output
			if len(o) > 1500 {
				o = o[:1500] + "\n... (truncated)"
			}
			userMessage.WriteString(fmt.Sprintf("\n--- Step %d ---\n%s\n", i+1, o))
		}
		userMessage.WriteString("\nWhat command should I run next? Remember to output ONLY JSON.")
	}

	response, err := t.llm.SimpleChat(ctx, systemPrompt, userMessage.String())
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse JSON response
	response = strings.TrimSpace(response)

	// Try to extract JSON from response (LLM might add extra text)
	var result DiscoveryResponse
	if err := json.Unmarshal([]byte(response), &result); err != nil {
		// Try to find JSON in response
		start := strings.Index(response, "{")
		end := strings.LastIndex(response, "}")
		if start != -1 && end > start {
			if err := json.Unmarshal([]byte(response[start:end+1]), &result); err != nil {
				return nil, fmt.Errorf("failed to parse LLM response as JSON: %s", response)
			}
		} else {
			return nil, fmt.Errorf("no JSON found in response: %s", response)
		}
	}

	return &result, nil
}

// executeDiscoveryCommand runs a command during discovery with proper environment
func (t *ShellTool) executeDiscoveryCommand(ctx context.Context, command string, tool *config.ExternalTool) string {
	// Notify observer of discovery command
	if t.observer != nil {
		t.observer(command, true)
	}

	cmd := exec.CommandContext(ctx, "sh", "-c", command)

	// Set environment variables
	if env := tool.BuildEnv(); env != nil {
		cmd.Env = env
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	_ = cmd.Run() // Ignore error - help commands often exit non-zero

	output := stdout.String()
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += stderr.String()
	}

	return output
}

// runSimpleDiscovery is the fallback when no LLM is available
func (t *ShellTool) runSimpleDiscovery(tool *config.ExternalTool, result *strings.Builder) string {
	baseCmd := tool.Access.Command

	result.WriteString("## Main command help\n")
	mainHelp := t.fetchSingleHelp(baseCmd, "")
	if mainHelp == "" {
		result.WriteString("Could not fetch help for main command.\n")
		return result.String()
	}
	result.WriteString(mainHelp)
	result.WriteString("\n")

	// Parse and show subcommands
	subcommands := t.parseSubcommands(mainHelp)
	if len(subcommands) > 0 {
		fmt.Fprintf(result, "\n## Available subcommands: %v\n", subcommands)
		result.WriteString("Run `<command> <subcommand> --help` to learn more about each.\n")
	}

	result.WriteString("\n=== Discovery complete. ===\n")
	return result.String()
}

// fetchSingleHelp tries to get help for a command or subcommand
func (t *ShellTool) fetchSingleHelp(baseCmd, subcommand string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try different help patterns (including just running the command which sometimes shows help)
	var patterns []string
	if subcommand != "" {
		patterns = []string{"--help", "-h", "help", "-help", ""}
	} else {
		patterns = []string{"--help", "-h", "help", "-help"}
	}

	for _, pattern := range patterns {
		var cmdStr string
		if subcommand != "" {
			if pattern == "" {
				cmdStr = fmt.Sprintf("%s %s", baseCmd, subcommand)
			} else {
				cmdStr = fmt.Sprintf("%s %s %s", baseCmd, subcommand, pattern)
			}
		} else {
			cmdStr = fmt.Sprintf("%s %s", baseCmd, pattern)
		}

		// Notify observer of discovery command
		if t.observer != nil {
			t.observer(cmdStr, true)
		}

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		_ = cmd.Run() // Ignore error - help often exits non-zero

		// Combine stdout and stderr - many tools output help to stderr
		output := stdout.String()
		if stderr.Len() > 0 {
			if output != "" {
				output += "\n"
			}
			output += stderr.String()
		}

		// Check if we got meaningful help - be more lenient
		if t.looksLikeHelp(output) {
			return output
		}
	}

	return ""
}

// looksLikeHelp checks if output appears to be help text
func (t *ShellTool) looksLikeHelp(output string) bool {
	if len(output) < 30 {
		return false
	}

	lower := strings.ToLower(output)

	// Common help indicators
	helpIndicators := []string{
		"usage:", "usage ", "options:", "commands:", "arguments:",
		"flags:", "subcommands:", "available commands",
		"--help", "-h,", "description:", "synopsis:",
		"positional arguments", "optional arguments",
		"examples:", "example:", "run '", "see '",
	}

	matches := 0
	for _, indicator := range helpIndicators {
		if strings.Contains(lower, indicator) {
			matches++
		}
	}

	// Need at least 1 strong indicator, or output is long enough to likely be help
	return matches >= 1 || len(output) > 200
}

// parseSubcommands attempts to extract subcommand names from help text
func (t *ShellTool) parseSubcommands(helpText string) []string {
	var subcommands []string
	lines := strings.Split(helpText, "\n")

	inCommandsSection := false

	for _, line := range lines {
		lineLower := strings.ToLower(line)

		// Detect commands/subcommands section
		if strings.Contains(lineLower, "commands:") ||
			strings.Contains(lineLower, "available commands") ||
			strings.Contains(lineLower, "subcommands:") {
			inCommandsSection = true
			continue
		}

		// End of commands section (empty line or new section)
		if inCommandsSection {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				// Could be end of section, but continue looking
				continue
			}

			// New section header (usually ends with ":" or starts with uppercase word followed by ":")
			if strings.HasSuffix(trimmed, ":") && !strings.Contains(trimmed, " ") {
				inCommandsSection = false
				continue
			}

			// Parse subcommand: typically "  subcommand    Description..."
			// or "  subcommand, alias   Description..."
			parts := strings.Fields(trimmed)
			if len(parts) >= 1 {
				cmd := parts[0]
				// Clean up: remove trailing comma, skip if it looks like a flag
				cmd = strings.TrimSuffix(cmd, ",")
				if !strings.HasPrefix(cmd, "-") && len(cmd) > 1 && len(cmd) < 30 && isValidSubcommand(cmd) {
					subcommands = append(subcommands, cmd)
				}
			}
		}
	}

	return subcommands
}

// isValidSubcommand checks if a string looks like a valid subcommand name
func isValidSubcommand(s string) bool {
	for _, r := range s {
		isLower := r >= 'a' && r <= 'z'
		isUpper := r >= 'A' && r <= 'Z'
		isDigit := r >= '0' && r <= '9'
		isSpecial := r == '-' || r == '_'
		if !isLower && !isUpper && !isDigit && !isSpecial {
			return false
		}
	}
	return true
}
