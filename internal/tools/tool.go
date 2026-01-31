package tools

// Tool represents a callable tool
type Tool interface {
	// Name returns the tool name
	Name() string

	// Description returns a description for the LLM
	Description() string

	// Parameters returns the JSON schema for parameters
	Parameters() map[string]any

	// Execute runs the tool with the given arguments
	Execute(args map[string]any) (string, error)
}

// Definition returns the Ollama tool definition format
func Definition(t Tool) map[string]any {
	return map[string]any{
		"type": "function",
		"function": map[string]any{
			"name":        t.Name(),
			"description": t.Description(),
			"parameters":  t.Parameters(),
		},
	}
}
