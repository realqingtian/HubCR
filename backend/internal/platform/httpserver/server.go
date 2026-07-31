package httpserver

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"
)

type Server struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	logger          *slog.Logger
}

func New(address string, shutdownTimeout time.Duration, handler http.Handler, logger *slog.Logger) *Server {
	return &Server{
		httpServer: &http.Server{
			Addr:              address,
			Handler:           handler,
			ReadHeaderTimeout: 5 * time.Second,
		},
		shutdownTimeout: shutdownTimeout,
		logger:          logger,
	}
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		s.logger.Info("control plane listening", "address", s.httpServer.Addr)
		errCh <- s.httpServer.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	}
}
