package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func TestMixedChainSessionAuthorizationAndReplace(t *testing.T) {
	var calls atomic.Int32
	session := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		assert.Equal(t, "", getHeader(r.Context.Request.Headers, "X-Auth-User"),
			"spoofed session header survived")

		return ok(&pb.ContinueResponse{HeaderMutation: set("X-Auth-User", "session-user"), ResponseHeaderMutation: set("X-Result", "first")}), nil
	})
	authz := restServer(t, func(input map[string]any) any {
		assert.Equal(t, config.RequestHeaders, input["eventType"],
			"wrong event")

		context := input["context"].(map[string]any)
		headers := context["request"].(map[string]any)["headers"].([]any)
		found := false
		for _, raw := range headers {
			h := raw.(map[string]any)
			if strings.EqualFold(h[jsonHeaderKey].(string), "X-Auth-User") && h["value"] == "session-user" {
				found = true
			}
		}
		assert.True(t, found,
			"authz did not see session")
		assert.Equal(t, "42", context["pathParameters"].(map[string]any)["id"],
			"missing path parameter")

		return map[string]any{"responseType": "continueResponse", "headerMutation": map[string]any{jsonSet: []any{map[string]any{jsonHeaderKey: "x-auth-user", jsonHeaderValues: []string{"authorized-user"}}}}, "responseHeaderMutation": map[string]any{jsonSet: []any{map[string]any{jsonHeaderKey: "X-Result", jsonHeaderValues: []string{"second"}}}}}
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		{
			vs := r.Header.Values("X-Auth-User")
			assert.EqualValues(t, 1, len(vs),
				"SetOrReplaceHeaders failed")
			assert.Equal(t, "authorized-user", vs[0],
				"SetOrReplaceHeaders failed")
		}

		w.Header().Set("X-Result", "upstream")
		_, _ = w.Write([]byte("ok"))
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/v1
internalHeaders: {headers: [X-Auth-User]}
---
kind: gateway/service/v1
name: session
interceptor: {address: '%s', transport: grpc}
---
kind: gateway/service/v1
name: authz
interceptor: {address: '%s', transport: rest}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/users/[^/]+$'
    target: '%s'
    interceptors:
      - id: session
      - id: authz
        pathParametersPattern: '^/users/{id}$'
`, session, authz.URL, backend.URL))
	status, h, body := request(t, s, "GET", "/users/42", "", http.Header{"X-Auth-User": []string{"spoofed"}})
	require.EqualValues(t, 200, status,
		"chain failed: %d %v %s", status, h, body)
	require.Equal(t, "ok", body,
		"chain failed: %d %v %s", status, h, body)
	require.Equal(t, "second", h.Get("X-Result"),
		"chain failed: %d %v %s", status, h, body)
	require.EqualValues(t, 1, calls.Load(),
		"chain failed: %d %v %s", status, h, body)
}
func TestAuthorizationStopsChainAndBackend(t *testing.T) {
	var called atomic.Int32
	deny := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		return &pb.InterceptResponse{Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 403, Body: []byte("denied")}}}, nil
	})
	backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called.Add(1) }))
	defer backend.Close()
	next := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		called.Add(1)
		return ok(&pb.ContinueResponse{}), nil
	})
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: deny
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: next
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    interceptors: [{id: deny}, {id: next}]
`, deny, next, backend.URL))
	status, _, body := request(t, s, "GET", "/", "", nil)
	require.EqualValues(t, 403, status,
		"denial did not stop processing")
	require.Equal(t, "denied", body,
		"denial did not stop processing")
	require.EqualValues(t, 0, called.Load(),
		"denial did not stop processing")
}
