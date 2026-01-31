package client

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/marcin/crabby/internal/api"
	"google.golang.org/protobuf/proto"
)

// Client handles communication with the daemon
type Client struct {
	baseURL string
	wsURL   string
}

// NewClient creates a new client
func NewClient(port int) *Client {
	return &Client{
		baseURL: fmt.Sprintf("http://localhost:%d", port),
		wsURL:   fmt.Sprintf("ws://localhost:%d", port),
	}
}

// Chat sends a message and streams the response to the provided writer
func (c *Client) Chat(ctx context.Context, message string, output io.Writer) error {
	conn, _, err := websocket.DefaultDialer.DialContext(ctx, c.wsURL+"/ws/chat", nil)
	if err != nil {
		return fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer conn.Close()

	// Send request
	req := &api.ChatRequest{
		Message: message,
	}
	data, err := proto.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}

	// Read streaming response
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, respData, err := conn.ReadMessage()
		if err != nil {
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				return nil
			}
			return fmt.Errorf("failed to read response: %w", err)
		}

		var resp api.ChatResponse
		if err := proto.Unmarshal(respData, &resp); err != nil {
			return fmt.Errorf("failed to unmarshal response: %w", err)
		}

		switch payload := resp.Payload.(type) {
		case *api.ChatResponse_Token:
			fmt.Fprint(output, payload.Token)
		case *api.ChatResponse_Done:
			fmt.Fprintln(output)
			return nil
		case *api.ChatResponse_Error:
			return fmt.Errorf("server error: %s", payload.Error)
		}
	}
}

// Status checks the daemon status
func (c *Client) Status(ctx context.Context) (*api.StatusResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/status", nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("daemon returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var status api.StatusResponse
	if err := proto.Unmarshal(data, &status); err != nil {
		return nil, err
	}

	return &status, nil
}

// IsRunning checks if the daemon is running
func (c *Client) IsRunning(ctx context.Context) bool {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}
