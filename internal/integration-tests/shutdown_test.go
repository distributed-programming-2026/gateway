package integration_tests

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/server"
)

func TestGracefulShutdownDrainsInflightRequest(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		_, _ = w.Write([]byte("finished"))
	}))
	defer backend.Close()
	c := load(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s'}]\n", backend.URL))
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
	client := &http.Client{Timeout: 3 * time.Second}
	waitStatus(t, client, "http://"+health.Addr().String()+"/readyz", 204)
	requestDone := make(chan string, 1)
	go func() {
		response, err := client.Get("http://" + proxy.Addr().String() + "/")
		if err != nil {
			requestDone <- err.Error()
			return
		}
		body, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			requestDone <- err.Error()
			return
		}
		requestDone <- string(body)
	}()
	<-started
	cancel()
	waitStatus(t, client, "http://"+health.Addr().String()+"/readyz", 503)
	close(release)
	{
		body := <-requestDone
		require.Equal(t, "finished", body,
			"inflight request canceled", body)
	}
	require.NoError(t, <-done)
}
