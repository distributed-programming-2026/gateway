package server

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestShutdownForceClosesAfterGracePeriod(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	srv := &http.Server{ReadHeaderTimeout: time.Second, Handler: http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		close(started)
		<-r.Context().Done()
		close(canceled)
	})}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = srv.Close() }()
	serveDone := make(chan error, 1)
	go func() { serveDone <- srv.Serve(listener) }()
	requestDone := make(chan error, 1)
	go func() {
		client := &http.Client{Timeout: 3 * time.Second}
		response, err := client.Get("http://" + listener.Addr().String())
		if response != nil {
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
		}
		requestDone <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "request did not reach server")
	}
	require.ErrorIs(t, shutdown([]*http.Server{srv}, 20*time.Millisecond), context.DeadlineExceeded)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		require.FailNow(t, "forced close did not cancel inflight request")
	}
	require.Error(t, <-requestDone)
	require.ErrorIs(t, <-serveDone, http.ErrServerClosed)
}
