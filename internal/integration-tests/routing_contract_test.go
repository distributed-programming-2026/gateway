package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuiltinsRunBeforeFinalRouteInterceptors(t *testing.T) {
	var intermediateCalls, finalCalls atomic.Int32
	intermediate := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		intermediateCalls.Add(1)
		return ok(&pb.ContinueResponse{}), nil
	})
	final := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		finalCalls.Add(1)
		assert.Equal(t, "/final/42", r.Context.Request.Path)
		assert.Equal(t, "42", r.Context.PathParameters["id"])
		assert.Equal(t, "keep=1", r.Context.Request.RawQuery)
		return ok(&pb.ContinueResponse{}), nil
	})
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: intermediate
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: final
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: routes
routes:
  - pattern: '^/start/'
    rewrite: {old: '^/start/(?P<id>[0-9]+)$', new: '/rewritten/${id}'}
    internalRedirect: {old: '^/rewritten/([0-9]+)$', new: '/end/$1'}
    interceptors: [{id: intermediate}]
  - pattern: '^/end/'
    rewrite: {old: '^/end/(.*)$', new: '/final/$1'}
    directResponse: {status: 200, inlineString: done}
    interceptors: [{id: final, pathParametersPattern: '^/final/{id}$'}]
`, intermediate, final))
	status, _, body := request(t, s, "GET", "/start/42?keep=1", "", nil)
	require.Equal(t, 200, status)
	require.Equal(t, "done", body)
	require.Zero(t, intermediateCalls.Load())
	require.EqualValues(t, 1, finalCalls.Load())
}
func TestPatternsHostAndHeaderConjunction(t *testing.T) {
	s := gateway(t, `kind: gateway/service/v1
name: routes
routes:
  - patterns: ['^/one$', '^/two$']
    headerMatch: [{header: Host, valueRegex: '^tenant\.example$'}, {header: X-Region, valueRegex: '^ru$'}]
    directResponse: {status: 200, inlineString: matched}
`)
	for _, tc := range []struct {
		path, host, region string
		want               int
	}{
		{firstPatternPath, tenantHost, "ru", 200}, {"/two", tenantHost, "ru", 200},
		{firstPatternPath, "wrong.example", "ru", 404}, {firstPatternPath, tenantHost, "en", 404}, {"/missing", tenantHost, "ru", 404},
	} {
		t.Run(tc.path+tc.host+tc.region, func(t *testing.T) {
			req, err := http.NewRequest("GET", s.URL+tc.path, http.NoBody)
			require.NoError(t, err)
			req.Host = tc.host
			req.Header.Set("X-Region", tc.region)
			response, err := s.Client().Do(req)
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			require.Equal(t, tc.want, response.StatusCode)
		})
	}
}
func TestAddHeadersSuccessAlwaysLiteralAndMismatch(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Always", "upstream")
		if r.URL.Path == "/error/42" {
			w.WriteHeader(500)
		} else {
			w.WriteHeader(200)
		}
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    addHeaders:
      X-Success: {value: success}
      X-Always: {value: local, always: true}
      X-Dynamic: {value: '{{literal}}={id}', pathParametersPattern: '^/(success|error)/{id}$', always: true}
      X-Mismatch: {value: '{id}', pathParametersPattern: '^/other/{id}$', always: true}
      X-Static: {value: '{literal}', always: true}
`, backend.URL))
	for _, path := range []string{"/success/42", "/error/42"} {
		status, h, _ := request(t, s, "GET", path, "", nil)
		require.Equal(t, "local", h.Get("X-Always"))
		require.Equal(t, "{literal}=42", h.Get("X-Dynamic"))
		require.Empty(t, h.Get("X-Mismatch"))
		require.Equal(t, "{literal}", h.Get("X-Static"))
		if status == 200 {
			require.Equal(t, "success", h.Get("X-Success"))
		} else {
			require.Equal(t, 500, status)
			require.Empty(t, h.Get("X-Success"))
		}
	}
}
func TestExternalErrorPagesSeeGatewayFailures(t *testing.T) {
	var observed atomic.Int32
	pages := grpcServer(t, func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		assert.Equal(t, "upstream_unavailable", r.Context.GatewayError.GetCode())
		assert.EqualValues(t, 502, *r.GetResponseHeaders().StatusCode)
		observed.Add(1)
		return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("external error page")}}), nil
	})
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: pages
interceptor: {address: '%s'}
---
kind: gateway/service/v1
name: backend
routes: [{pattern: '^/', target: '127.0.0.1:1', interceptors: [{id: pages, processingMode: {requestHeaders: false, responseHeaders: true}}]}]
`, pages))
	status, _, body := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 502, status)
	require.Equal(t, "external error page", body)
	require.EqualValues(t, 1, observed.Load())
}
