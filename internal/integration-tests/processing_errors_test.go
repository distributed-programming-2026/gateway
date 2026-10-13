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

func TestBufferedResponseReadErrors(t *testing.T) {
	for _, scenario := range []string{"incomplete", timeoutScenario} {
		t.Run(scenario, func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Length", "100")
				_, _ = w.Write([]byte("partial"))
				w.(http.Flusher).Flush()
				if scenario == timeoutScenario {
					<-r.Context().Done()
				}
			}))
			defer backend.Close()
			processor := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
				return ok(&pb.ContinueResponse{}), nil
			}
			s := interceptedGateway(t, backend.URL, "{responseBody: buffered}", "    timeout: 0s\n    idleTimeout: 30ms", []string{grpcTransport}, processor)
			status, _, _ := request(t, s, "GET", "/", "", nil)
			want := 502
			if scenario == timeoutScenario {
				want = 504
			}
			require.Equal(t, want, status)
		})
	}
}
func TestFailedLocalResponseProcessorIsNotRetried(t *testing.T) {
	processor := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		return nil, fmt.Errorf("processor unavailable")
	})
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s'}\n---\nkind: gateway/service/v1\nname: local\nroutes: [{pattern: '^/', directResponse: {status: 200}, interceptors: [{id: processor, processingMode: {requestHeaders: false, responseHeaders: true}}]}]\n", processor))
	status, _, _ := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 502, status)
}
func TestUpgradeBodyProcessingRejected(t *testing.T) {
	for _, buffered := range []bool{false, true} {
		t.Run(fmt.Sprint(buffered), func(t *testing.T) {
			backend := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { assert.Fail(t, "upgrade should not reach upstream") }))
			defer backend.Close()
			f := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
				if buffered {
					return ok(&pb.ContinueResponse{}), nil
				}
				return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("replacement")}}), nil
			}
			mode := "{}"
			if buffered {
				mode = "{requestBody: buffered}"
			}
			s := interceptedGateway(t, backend.URL, mode, "", []string{grpcTransport}, f)
			status, _, _ := request(t, s, "GET", "/", "", http.Header{"Connection": []string{"Upgrade"}, "Upgrade": []string{"echo"}})
			require.Equal(t, 502, status)
		})
	}
}
func TestPendingHeadersAppliedToImmediateAndOverridden(t *testing.T) {
	first := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		return ok(&pb.ContinueResponse{ResponseHeaderMutation: set("X-Previous", "previous")}), nil
	}
	second := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		return &pb.InterceptResponse{Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 403, Headers: []*pb.Header{{Key: "X-Previous", Value: "immediate"}, {Key: "Set-Cookie", Value: "a=1"}, {Key: "Set-Cookie", Value: "b=2"}}}}}, nil
	}
	s := interceptedGateway(t, "127.0.0.1:1", "{}", "", []string{grpcTransport, restTransport}, first, second)
	status, h, _ := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 403, status)
	require.Equal(t, []string{"immediate"}, h.Values("X-Previous"))
	require.Equal(t, []string{"a=1", "b=2"}, h.Values("Set-Cookie"))
}
func TestNoMatchingInternalRedirectHasNoImplicitTarget(t *testing.T) {
	s := gateway(t, "kind: gateway/service/v1\nname: routes\nroutes: [{pattern: '^/', internalRedirect: {old: '^/other$', new: /final}}]\n")
	status, _, _ := request(t, s, "GET", "/missing", "", nil)
	require.Equal(t, 404, status)
}
func TestRESTTimeoutCancelsProcessor(t *testing.T) {
	canceled := make(chan struct{})
	processor := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
		close(canceled)
	}))
	defer processor.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s', transport: rest, timeout: 30ms}\n---\nkind: gateway/service/v1\nname: local\nroutes: [{pattern: '^/', directResponse: {status: 200}, interceptors: [{id: processor}]}]\n", processor.URL))
	status, _, _ := request(t, s, "GET", "/", "", nil)
	require.Equal(t, 504, status)
	select {
	case <-canceled:
	case <-time.After(time.Second):
		require.FailNow(t, "REST timeout did not cancel processor")
	}
}
