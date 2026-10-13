package integration_tests

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/require"
)

func TestInvalidRequestDecisionsFailClosed(t *testing.T) {
	badMethod, badPath, badAuthority, badQuery := "bad method", "relative", "user@host", "?query"
	tests := map[string]*pb.InterceptResponse{
		"empty":             {},
		"status-phase":      ok(&pb.ContinueResponse{StatusCode: ptr(int32(201))}),
		"header-name":       ok(&pb.ContinueResponse{HeaderMutation: set("bad header", "x")}),
		"header-value":      ok(&pb.ContinueResponse{HeaderMutation: set("X-Test", "a\r\nb")}),
		"framing":           ok(&pb.ContinueResponse{HeaderMutation: set("Content-Length", "10")}),
		"remove-host":       ok(&pb.ContinueResponse{HeaderMutation: &pb.HeaderMutation{Remove: []string{"Host"}}}),
		"empty-values":      ok(&pb.ContinueResponse{HeaderMutation: set("X-Test")}),
		"duplicate-set":     ok(&pb.ContinueResponse{HeaderMutation: &pb.HeaderMutation{Set: []*pb.HeaderValues{{Key: "X-Test", Values: []string{"a"}}, {Key: "x-test", Values: []string{"b"}}}}}),
		"method":            ok(&pb.ContinueResponse{RequestMutation: &pb.RequestMutation{Method: &badMethod}}),
		"path":              ok(&pb.ContinueResponse{RequestMutation: &pb.RequestMutation{Path: &badPath}}),
		"authority":         ok(&pb.ContinueResponse{RequestMutation: &pb.RequestMutation{Authority: &badAuthority}}),
		"query":             ok(&pb.ContinueResponse{RequestMutation: &pb.RequestMutation{RawQuery: &badQuery}}),
		"trailer-phase":     ok(&pb.ContinueResponse{TrailerMutation: set("X-Trailer", "x")}),
		"replacement-limit": ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("too large")}}),
		"immediate-limit":   {Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 403, Body: []byte("too large")}}},
		"immediate-status":  {Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 100}}},
		"immediate-header":  {Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 403, Headers: []*pb.Header{{Key: "Transfer-Encoding", Value: "chunked"}}}}},
	}
	for name, decision := range tests {
		t.Run(name, func(t *testing.T) {
			var backendCalls atomic.Int32
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { backendCalls.Add(1); w.WriteHeader(204) }))
			defer backend.Close()
			f := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) { return decision, nil }
			s := interceptedGateway(t, backend.URL, "{maxRequestBodyBytes: 3, maxResponseBodyBytes: 3}", "", []string{grpcTransport}, f)
			status, _, _ := request(t, s, "GET", "/", "", nil)
			require.Equal(t, 502, status)
			require.Zero(t, backendCalls.Load())
		})
	}
}
func TestRESTTransportFailuresFailClosed(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"transport-status", 503, "unavailable"}, {"invalid-json", 200, "{"},
		{"missing-payload", 200, `{}`}, {"invalid-body-base64", 200, `{"payload":{"responseType":"continueResponse","bodyMutation":{"data":"%%%invalid%%%"}}}`},
		{"unknown-decision", 200, `{"payload":{"responseType":"unknown"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			processor := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer processor.Close()
			s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s', transport: rest}\n---\nkind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', directResponse: {status: 200}, interceptors: [{id: processor}]}]\n", processor.URL))
			status, _, _ := request(t, s, "GET", "/", "", nil)
			require.Equal(t, 502, status)
		})
	}
}
func ptr[T any](value T) *T { return &value }
