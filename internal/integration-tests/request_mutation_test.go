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

func TestRequestMetadataMutationEmptyBodyAndHeaderOperations(t *testing.T) {
	processor := grpcServer(t, func(_ context.Context, _ *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		method := "PUT"
		path := "/modified"
		query := "a=1&a=2"
		authority := "preserved.example"
		return ok(&pb.ContinueResponse{RequestMutation: &pb.RequestMutation{Method: &method, Path: &path, RawQuery: &query, Authority: &authority}, HeaderMutation: &pb.HeaderMutation{Remove: []string{"X-Remove"}, Set: []*pb.HeaderValues{{Key: multiHeader, Values: []string{jsonSet}}}, Append: []*pb.HeaderValues{{Key: multiHeader, Values: []string{"append"}}}}, BodyMutation: &pb.BodyMutation{Data: []byte{}}}), nil
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		assert.Equal(t, "PUT", r.Method,
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)
		assert.Equal(t, "/modified?a=1&a=2", r.URL.RequestURI(),
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)
		assert.Equal(t, "preserved.example", r.Host,
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)
		assert.EqualValues(t, 0, len(b),
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)
		assert.Equal(t, "", r.Header.Get("X-Remove"),
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)
		assert.Equal(t, "set,append", strings.Join(r.Header.Values(multiHeader), ","),
			"incorrect mutations: %s %s %s %q %v", r.Method, r.URL, r.Host, b, r.Header)

		w.WriteHeader(204)
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: processor
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/original$'
    target: '%s'
    disableHostRewrite: true
    interceptors: [{id: processor}]
`, processor, backend.URL))
	status, _, _ := request(t, s, "POST", "/original?old=true", "body", http.Header{"X-Remove": []string{"remove"}, multiHeader: []string{"old"}})
	require.EqualValues(t, 204, status,
		"metadata mutation failed")
}
