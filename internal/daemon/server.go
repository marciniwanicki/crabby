package daemon

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
	"github.com/marcin/crabby/internal/api"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
)

const Version = "0.1.0"

// Server represents the daemon server
type Server struct {
	port     int
	ollama   *OllamaClient
	handler  *Handler
	logger   zerolog.Logger
	upgrader websocket.Upgrader
}

// NewServer creates a new daemon server
func NewServer(port int, ollamaURL, model string) *Server {
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()
	ollama := NewOllamaClient(ollamaURL, model)
	handler := NewHandler(ollama, logger)

	return &Server{
		port:    port,
		ollama:  ollama,
		handler: handler,
		logger:  logger,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow local connections
			},
		},
	}
}

// Run starts the server and blocks until shutdown
func (s *Server) Run() error {
	mux := http.NewServeMux()

	// HTTP endpoints
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/status", s.handleStatus)

	// WebSocket endpoints
	mux.HandleFunc("/ws/chat", s.handleWSChat)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.port),
		Handler: mux,
	}

	// Graceful shutdown
	done := make(chan bool)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		s.logger.Info().Msg("shutting down server...")

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := server.Shutdown(ctx); err != nil {
			s.logger.Error().Err(err).Msg("server shutdown error")
		}
		close(done)
	}()

	s.logger.Info().
		Int("port", s.port).
		Str("model", s.ollama.Model()).
		Msg("starting daemon server")

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		return err
	}

	<-done
	s.logger.Info().Msg("server stopped")
	return nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	healthy, _ := s.ollama.Health(ctx)

	resp := &api.StatusResponse{
		Healthy: healthy,
		Model:   s.ollama.Model(),
		Version: Version,
	}

	data, err := proto.Marshal(resp)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-protobuf")
	w.Write(data)
}

func (s *Server) handleWSChat(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error().Err(err).Msg("failed to upgrade connection")
		return
	}

	s.logger.Info().Str("remote", r.RemoteAddr).Msg("new chat connection")
	s.handler.HandleChat(conn)
}
