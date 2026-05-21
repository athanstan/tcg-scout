package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"tcg-scout/internal/app"
)

type Server struct {
	service         *app.Service
	addr            string
	readTimeout     time.Duration
	writeTimeout    time.Duration
	idleTimeout     time.Duration
	shutdownTimeout time.Duration
}

type Option func(*Server) error

func WithAddress(addr string) Option {
	return func(s *Server) error {
		if strings.TrimSpace(addr) == "" {
			return fmt.Errorf("server address is required")
		}
		s.addr = addr
		return nil
	}
}

func WithReadTimeout(timeout time.Duration) Option {
	return func(s *Server) error {
		if timeout <= 0 {
			return fmt.Errorf("read timeout must be positive")
		}
		s.readTimeout = timeout
		return nil
	}
}

func WithWriteTimeout(timeout time.Duration) Option {
	return func(s *Server) error {
		if timeout <= 0 {
			return fmt.Errorf("write timeout must be positive")
		}
		s.writeTimeout = timeout
		return nil
	}
}

func WithIdleTimeout(timeout time.Duration) Option {
	return func(s *Server) error {
		if timeout <= 0 {
			return fmt.Errorf("idle timeout must be positive")
		}
		s.idleTimeout = timeout
		return nil
	}
}

func WithShutdownTimeout(timeout time.Duration) Option {
	return func(s *Server) error {
		if timeout <= 0 {
			return fmt.Errorf("shutdown timeout must be positive")
		}
		s.shutdownTimeout = timeout
		return nil
	}
}

func NewServer(service *app.Service, opts ...Option) (*Server, error) {
	if service == nil {
		return nil, fmt.Errorf("service is required")
	}

	server := &Server{
		service:         service,
		addr:            ":8080",
		readTimeout:     5 * time.Second,
		writeTimeout:    30 * time.Second,
		idleTimeout:     60 * time.Second,
		shutdownTimeout: 10 * time.Second,
	}
	for _, opt := range opts {
		if err := opt(server); err != nil {
			return nil, err
		}
	}

	return server, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /v1/games", s.handleGames)
	mux.HandleFunc("GET /v1/games/{game}/resources", s.handleResources)
	mux.HandleFunc("GET /v1/games/{game}/resources/{resource}/scrapers", s.handleScrapers)
	mux.HandleFunc("POST /v1/runs", s.handleRuns)
	return mux
}

func (s *Server) Run(ctx context.Context) error {
	server := &http.Server{
		Addr:         s.addr,
		Handler:      s.Handler(),
		ReadTimeout:  s.readTimeout,
		WriteTimeout: s.writeTimeout,
		IdleTimeout:  s.idleTimeout,
	}

	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}

	errCh := make(chan error, 1)
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown: %w", err)
		}
		return <-errCh
	}
}

type runRequest struct {
	Game     string      `json:"game"`
	Resource string      `json:"resource"`
	Scraper  string      `json:"scraper"`
	Action   string      `json:"action"`
	Options  app.Request `json:"options"`
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleGames(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"games": s.service.Games()})
}

func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	game := r.PathValue("game")
	resources := s.service.Resources(game)
	if resources == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown game %q", game)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"game": game, "resources": resources})
}

func (s *Server) handleScrapers(w http.ResponseWriter, r *http.Request) {
	game := r.PathValue("game")
	resource := r.PathValue("resource")
	scrapers := s.service.Scrapers(game, resource)
	if scrapers == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": fmt.Sprintf("unknown resource %q for game %q", resource, game)})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"game":     game,
		"resource": resource,
		"scrapers": scrapers,
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	request := runRequest{
		Options: app.DefaultRequest(),
	}
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("decode request: %v", err)})
		return
	}

	result, err := s.service.Execute(r.Context(), app.Selection{
		Game:     request.Game,
		Resource: request.Resource,
		Scraper:  request.Scraper,
	}, request.Action, request.Options)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	_ = encoder.Encode(value)
}
