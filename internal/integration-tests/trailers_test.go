package integration_tests

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func TestTrailerMutationsReachNextProcessorAndBothPeers(t *testing.T) {
	first := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		if r.GetRequestTrailers() != nil {
			assert.Equal(t, "original", getHeader(r.GetRequestTrailers().Trailers, "X-Upload-Trailer"))
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Upload-Trailer", "changed")}), nil
		}
		if r.GetResponseTrailers() != nil {
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Download-Trailer", "changed")}), nil
		}
		return ok(&pb.ContinueResponse{}), nil
	})
	second := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		assert.False(t, r.GetRequestTrailers() != nil && getHeader(r.GetRequestTrailers().Trailers, "X-Upload-Trailer") != "changed",
			"next processor missed request trailer mutation")
		assert.False(t, r.GetResponseTrailers() != nil && getHeader(r.GetResponseTrailers().Trailers, "X-Download-Trailer") != "changed",
			"next processor missed response trailer mutation")

		return ok(&pb.ContinueResponse{}), nil
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		assert.Equal(t, "changed", r.Trailer.Get("X-Upload-Trailer"),
			"upstream trailers: %v", r.Trailer)

		w.Header().Set("Trailer", "X-Download-Trailer")
		_, _ = w.Write([]byte("body"))
		w.Header().Set("X-Download-Trailer", "original")
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
        processingMode: {requestBody: buffered, responseBody: buffered, requestTrailers: true, responseTrailers: true}
      - id: second
        processingMode: {requestBody: buffered, responseBody: buffered, requestTrailers: true, responseTrailers: true}
`, first, second, backend.URL))
	req, _ := http.NewRequest("POST", s.URL, strings.NewReader("body"))
	req.ContentLength = -1
	req.Trailer = http.Header{"X-Upload-Trailer": []string{"original"}}
	response, err := s.Client().Do(req)
	require.NoError(t, err)

	_, err = io.ReadAll(response.Body)
	_ = response.Body.Close()
	require.NoError(t, err)
	require.Equal(t, "changed", response.Trailer.Get("X-Download-Trailer"),
		"client trailers %v", response.Trailer)
}

func TestStreamingRequestTrailersWithoutProcessing(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "upload", string(body))
		assert.Equal(t, "original", r.Trailer.Get("X-Upload"))
		w.WriteHeader(204)
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s'}]\n", backend.URL))
	req, err := http.NewRequest("POST", s.URL, strings.NewReader("upload"))
	require.NoError(t, err)
	req.ContentLength = -1
	req.Trailer = http.Header{"X-Upload": []string{"original"}}
	response, err := s.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, 204, response.StatusCode)
}
