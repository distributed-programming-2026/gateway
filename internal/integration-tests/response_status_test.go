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

func TestResponseStatusMutationRemovesForbiddenBody(t *testing.T) {
	for _, want := range []int32{204, 304} {
		t.Run(fmt.Sprint(want), func(t *testing.T) {
			processor := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
				status := want
				return ok(&pb.ContinueResponse{StatusCode: &status}), nil
			})
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("must not be sent")) }))
			defer backend.Close()
			s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: processor
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    interceptors:
      - id: processor
        processingMode: {requestHeaders: false, responseHeaders: true}
`, processor, backend.URL))
			status, _, body := request(t, s, "GET", "/", "", nil)
			require.EqualValues(t, want, status,
				"body sent with bodyless status")
			require.Equal(t, "", body,
				"body sent with bodyless status")
		})
	}
}
