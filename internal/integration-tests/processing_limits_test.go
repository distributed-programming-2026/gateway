package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func TestLimitsAndInvalidInterceptorResponses(t *testing.T) {
	for _, scenario := range []string{"request-limit", responseLimitScenario, timeoutScenario, "invalid-phase", "malformed-rest"} {
		t.Run(scenario, func(t *testing.T) {
			var calls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = w.Write([]byte("large-response")) }))
			defer backend.Close()
			address := grpcServer(t, func(ctx context.Context, _ *pb.InterceptRequest) (*pb.InterceptResponse, error) {
				if scenario == timeoutScenario {
					<-ctx.Done()
					return nil, ctx.Err()
				}
				if scenario == "invalid-phase" {
					code := int32(201)
					return ok(&pb.ContinueResponse{StatusCode: &code}), nil
				}
				return ok(&pb.ContinueResponse{}), nil
			})
			transport := grpcTransport
			mode := "{}"
			timeout := "1s"
			want := 502
			if scenario == "request-limit" {
				mode = "{requestBody: buffered, maxRequestBodyBytes: 3}"
				want = 413
			}
			if scenario == responseLimitScenario {
				mode = "{responseBody: buffered, maxResponseBodyBytes: 3}"
			}
			if scenario == timeoutScenario {
				timeout = "20ms"
				want = 504
			}
			if scenario == "malformed-rest" {
				rest := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"payload":{"responseType":"continueResponse","unknown":true}}`))
				}))
				defer rest.Close()
				address = rest.URL
				transport = restTransport
			}
			s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: processor
interceptor: {address: '%s', transport: %s, timeout: %s}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    interceptors:
      - id: processor
        processingMode: %s
`, address, transport, timeout, backend.URL, mode))
			status, _, _ := request(t, s, "POST", "/", "body", nil)
			require.Equal(t, want, status,
				"status %d want %d", status, want)
			require.False(t, scenario != responseLimitScenario && calls.Load() != 0,
				"backend called after failed request processing")
		})
	}
}
