package integration_tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGlobalTimeoutRouteOverrideAndExplicitDisable(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(120 * time.Millisecond):
			_, _ = w.Write([]byte("done"))
		case <-r.Context().Done():
		}
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/v1
proxyTimeouts: {timeout: 30ms, idleTimeout: 30ms}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/global$'
    target: '%s'
  - pattern: '^/disabled$'
    target: '%s'
    timeout: 0s
    idleTimeout: 0s
  - pattern: '^/override$'
    target: '%s'
    timeout: 1s
    idleTimeout: 1s
`, backend.URL, backend.URL, backend.URL))
	for _, tc := range []struct {
		path string
		want int
	}{{"/global", 504}, {"/disabled", 200}, {"/override", 200}} {
		status, _, body := request(t, s, "GET", tc.path, "", nil)
		require.Equal(t, tc.want, status)
		if tc.want == 200 {
			require.Equal(t, "done", body)
		}
	}
}
func TestIdleTimeoutWaitsForUploadCompletion(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "slow upload", string(body))
		_, _ = w.Write([]byte("done"))
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s', timeout: 0s, idleTimeout: 150ms}]\n", backend.URL))
	reader, writer := io.Pipe()
	req, err := http.NewRequest("POST", s.URL, reader)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := writer.Write([]byte("slow "))
		if err == nil {
			time.Sleep(300 * time.Millisecond)
			_, err = writer.Write([]byte("upload"))
		}
		_ = writer.CloseWithError(err)
		done <- err
	}()
	response, err := s.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	body, err := io.ReadAll(response.Body)
	require.NoError(t, err)
	require.NoError(t, <-done)
	require.Equal(t, 200, response.StatusCode)
	require.Equal(t, "done", string(body))
}
func TestInterceptorTimeoutExplicitDisableAndCancellation(t *testing.T) {
	for _, transport := range []string{grpcTransport, restTransport} {
		t.Run(transport, func(t *testing.T) {
			address := processorServer(t, transport, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
				time.Sleep(50 * time.Millisecond)
				return ok(&pb.ContinueResponse{}), nil
			})
			s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s', transport: %s, timeout: 0s}\n---\nkind: gateway/service/v1\nname: local\nroutes: [{pattern: '^/', directResponse: {status: 200, inlineString: ok}, interceptors: [{id: processor}]}]\n", address, transport))
			status, _, body := request(t, s, "GET", "/", "", nil)
			require.Equal(t, 200, status)
			require.Equal(t, "ok", body)
		})
	}
	started := make(chan struct{})
	canceled := make(chan struct{})
	address := grpcServer(t, func(ctx context.Context, _ *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		close(started)
		<-ctx.Done()
		close(canceled)
		return nil, ctx.Err()
	})
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s', timeout: 0s}\n---\nkind: gateway/service/v1\nname: local\nroutes: [{pattern: '^/', directResponse: {status: 200}, interceptors: [{id: processor}]}]\n", address))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", s.URL, http.NoBody)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		response, err := s.Client().Do(req)
		if response != nil {
			_ = response.Body.Close()
		}
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "interceptor did not start")
	}
	cancel()
	select {
	case <-canceled:
	case <-time.After(time.Second):
		require.FailNow(t, "interceptor did not receive cancellation")
	}
	require.Error(t, <-done)
}
