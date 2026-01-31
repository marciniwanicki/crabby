package daemon

import (
	"context"
	"io"
	"strings"

	"github.com/gorilla/websocket"
	"github.com/marcin/crabby/internal/api"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

// Handler manages WebSocket connections and message handling
type Handler struct {
	ollama *OllamaClient
	logger zerolog.Logger
}

// NewHandler creates a new handler
func NewHandler(ollama *OllamaClient, logger zerolog.Logger) *Handler {
	return &Handler{
		ollama: ollama,
		logger: logger,
	}
}

// HandleChat processes a chat WebSocket connection
func (h *Handler) HandleChat(conn *websocket.Conn) {
	defer conn.Close()

	for {
		messageType, data, err := conn.ReadMessage()
		if err != nil {
			// Treat EOF, unexpected EOF, and normal close as clean disconnects
			if websocket.IsCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) ||
				err == io.EOF || strings.Contains(err.Error(), "EOF") {
				h.logger.Debug().Msg("client disconnected")
			} else {
				h.logger.Error().Err(err).Msg("failed to read message")
			}
			return
		}

		if messageType != websocket.BinaryMessage {
			h.logger.Warn().Int("type", messageType).Msg("received non-binary message")
			continue
		}

		var req api.ChatRequest
		if err := proto.Unmarshal(data, &req); err != nil {
			h.logger.Error().Err(err).Msg("failed to unmarshal request")
			h.sendError(conn, "invalid request format")
			continue
		}

		h.logger.Info().Str("message", req.Message).Msg("received chat request")

		if err := h.processChat(conn, req.Message); err != nil {
			h.logger.Error().Err(err).Msg("failed to process chat")
			h.sendError(conn, err.Error())
		}
	}
}

func (h *Handler) processChat(conn *websocket.Conn, message string) error {
	ctx := context.Background()
	tokenChan := make(chan string, 100)

	errChan := make(chan error, 1)
	go func() {
		errChan <- h.ollama.Chat(ctx, message, tokenChan)
	}()

	// Stream tokens to client
	for token := range tokenChan {
		resp := &api.ChatResponse{
			Payload: &api.ChatResponse_Token{Token: token},
		}
		if err := h.sendResponse(conn, resp); err != nil {
			return err
		}
	}

	// Check for errors from the chat goroutine
	if err := <-errChan; err != nil {
		return err
	}

	// Send done signal
	resp := &api.ChatResponse{
		Payload: &api.ChatResponse_Done{Done: true},
	}
	return h.sendResponse(conn, resp)
}

func (h *Handler) sendResponse(conn *websocket.Conn, resp *api.ChatResponse) error {
	data, err := proto.Marshal(resp)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.BinaryMessage, data)
}

func (h *Handler) sendError(conn *websocket.Conn, errMsg string) {
	resp := &api.ChatResponse{
		Payload: &api.ChatResponse_Error{Error: errMsg},
	}
	data, err := proto.Marshal(resp)
	if err != nil {
		h.logger.Error().Err(err).Msg("failed to marshal error response")
		return
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, data); err != nil {
		h.logger.Error().Err(err).Msg("failed to send error response")
	}
}
