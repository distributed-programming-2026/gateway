package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCancellationReachesUpstream(t *testing.T) {
	started := make(chan struct{})
	canceled := make(chan struct{})
	backend := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) }))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes:\n  - pattern: '^/'\n    target: '%s'\n", backend.URL))
	ctx, cancel := context.WithCancel(context.Background())
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, http.NoBody)
	done := make(chan struct{})
	go func() {
		response, _ := s.Client().Do(req)
		if response != nil {
			_ = response.Body.Close()
		}
		close(done)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "upstream not reached")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		require.FailNow(t, "cancellation lost")
	}
	<-done
}
