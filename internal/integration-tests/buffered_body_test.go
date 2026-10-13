package integration_tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func TestBufferedRequestResponseAndTrailers(t *testing.T) {
	first := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		switch {
		case r.GetRequestBody() != nil:
			assert.Equal(t, "original", string(r.GetRequestBody().Data),
				"missing request body")

			return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("changed\x00")}}), nil
		case r.GetResponseBody() != nil:
			return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("result\x00")}}), nil
		default:
			return ok(&pb.ContinueResponse{}), nil
		}
	})
	second := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		assert.False(t, r.GetRequestBody() != nil && string(r.GetRequestBody().Data) != "changed\x00",
			"next processor lost request mutation")
		assert.False(t, r.GetResponseBody() != nil && string(r.GetResponseBody().Data) != "result\x00",
			"next processor lost response mutation")

		return ok(&pb.ContinueResponse{}), nil
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		assert.Equal(t, "changed\x00", string(b),
			"backend body %q length %d", b, r.ContentLength)
		assert.EqualValues(t, 8, r.ContentLength,
			"backend body %q length %d", b, r.ContentLength)

		w.Header().Set("ETag", "obsolete")
		_, _ = w.Write([]byte("upstream"))
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: first
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: second
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    interceptors:
      - id: first
        processingMode: {requestBody: buffered, responseBody: buffered}
      - id: second
        processingMode: {requestBody: buffered, responseBody: buffered}
`, first, second, backend.URL))
	status, h, b := request(t, s, "POST", "/", "original", nil)
	require.EqualValues(t, 200, status,
		"buffered mutations failed %d %v %q", status, h, b)
	require.Equal(t, "result\x00", b,
		"buffered mutations failed %d %v %q", status, h, b)
	require.Equal(t, "", h.Get("ETag"),
		"buffered mutations failed %d %v %q", status, h, b)
}
