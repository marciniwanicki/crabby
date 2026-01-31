package daemon

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// OllamaClient handles communication with the Ollama API
type OllamaClient struct {
	baseURL    string
	model      string
	httpClient *http.Client
}

// OllamaRequest represents a chat request to Ollama
type OllamaRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

// OllamaMessage represents a message in the Ollama chat format
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaResponse represents a streaming response from Ollama
type OllamaResponse struct {
	Model     string        `json:"model"`
	Message   OllamaMessage `json:"message"`
	Done      bool          `json:"done"`
	Error     string        `json:"error,omitempty"`
	CreatedAt string        `json:"created_at"`
}

// NewOllamaClient creates a new Ollama client
func NewOllamaClient(baseURL, model string) *OllamaClient {
	return &OllamaClient{
		baseURL:    baseURL,
		model:      model,
		httpClient: &http.Client{},
	}
}

// Chat sends a message to Ollama and streams the response
func (c *OllamaClient) Chat(ctx context.Context, message string, tokenChan chan<- string) error {
	defer close(tokenChan)

	req := OllamaRequest{
		Model: c.model,
		Messages: []OllamaMessage{
			{Role: "user", Content: message},
		},
		Stream: true,
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned status %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var ollamaResp OllamaResponse
		if err := json.Unmarshal(line, &ollamaResp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		if ollamaResp.Error != "" {
			return fmt.Errorf("ollama error: %s", ollamaResp.Error)
		}

		if ollamaResp.Message.Content != "" {
			tokenChan <- ollamaResp.Message.Content
		}

		if ollamaResp.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading response: %w", err)
	}

	return nil
}

// Health checks if Ollama is healthy and the model is available
func (c *OllamaClient) Health(ctx context.Context) (bool, error) {
	httpReq, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/tags", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// Model returns the configured model name
func (c *OllamaClient) Model() string {
	return c.model
}
