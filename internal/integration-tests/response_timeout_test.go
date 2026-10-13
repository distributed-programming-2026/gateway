package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func TestProxyTimeoutIncludesResponseProcessor(t *testing.T) {
	processor := grpcServer(t, func(ctx context.Context, _ *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) }))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: processor
interceptor: {address: '%s', timeout: 0s}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    timeout: 30ms
    interceptors:
      - id: processor
        processingMode: {requestHeaders: false, responseHeaders: true}
`, processor, backend.URL))
	status, _, _ := request(t, s, "GET", "/", "", nil)
	require.EqualValues(t, 504, status,
		"response processing escaped proxy deadline: %d", status)
}
