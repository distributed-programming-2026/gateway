package integration_tests

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/server"
)

func TestReadinessAndGracefulShutdown(t *testing.T) {
	c := load(t, `kind: gateway/service/v1
name: local
routes: [{pattern: '^/', directResponse: {status: 200, inlineString: ok}}]
`)
	r, err := server.New(c)
	require.NoError(t, err)

	defer func() { _ = r.Close() }()
	proxy, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	health, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- r.Serve(ctx, proxy, health) }()
	client := &http.Client{Timeout: time.Second}
	waitStatus(t, client, "http://"+health.Addr().String()+"/readyz", 204)
	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)

	case <-time.After(3 * time.Second):
		require.FailNow(t, "shutdown blocked")
	}
}
func waitStatus(t *testing.T, client *http.Client, address string, status int) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(address)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == status {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	require.FailNow(t, fmt.Sprintf("%s did not become ready", address))
}
