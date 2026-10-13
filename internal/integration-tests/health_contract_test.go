package integration_tests

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/server"
	"github.com/stretchr/testify/require"
)

func TestHealthReadinessAndMultipleListeners(t *testing.T) {
	r, err := server.New(load(t, "kind: gateway/v1\n"))
	require.NoError(t, err)
	defer func() { _ = r.Close() }()
	for _, tc := range []struct {
		path string
		want int
	}{{"/healthz", 204}, {"/readyz", 503}, {"/unknown", 404}} {
		rec := httptest.NewRecorder()
		r.Health.ServeHTTP(rec, httptest.NewRequest("GET", tc.path, http.NoBody))
		require.Equal(t, tc.want, rec.Code)
	}
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	health, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	alias, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx, proxy, health, alias) }()
	client := &http.Client{Timeout: time.Second}
	for _, l := range []net.Listener{health, alias} {
		waitStatus(t, client, "http://"+l.Addr().String()+"/readyz", 204)
		waitStatus(t, client, "http://"+l.Addr().String()+"/healthz", 204)
	}
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		require.FailNow(t, "shutdown did not stop all listeners")
	}
	rec := httptest.NewRecorder()
	r.Health.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", http.NoBody))
	require.Equal(t, 503, rec.Code)
}
