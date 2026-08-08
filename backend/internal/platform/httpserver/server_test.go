package httpserver

import (
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"
)

func TestNewConfiguresCompleteConnectionLimits(t *testing.T) {
	server := New(
		"127.0.0.1:0",
		10*time.Second,
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	configured := server.httpServer
	if configured.ReadHeaderTimeout != readHeaderTimeout ||
		configured.ReadTimeout != readTimeout ||
		configured.WriteTimeout != writeTimeout ||
		configured.IdleTimeout != idleTimeout ||
		configured.MaxHeaderBytes != maxHeaderBytes {
		t.Fatalf("http server limits = %#v", configured)
	}
}
