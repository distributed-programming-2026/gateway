package integration_tests

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImmediateResponseStopsEveryPhaseAndTransport(t *testing.T) {
	phases := []string{config.RequestHeaders, config.RequestBody, config.RequestTrailers, config.ResponseHeaders, config.ResponseBody, config.ResponseTrailers}
	for _, transport := range []string{grpcTransport, restTransport} {
		for _, phase := range phases {
			t.Run(transport+"/"+phase, func(t *testing.T) {
				var denied atomic.Bool
				var backendCalls, afterDenial atomic.Int32
				backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					backendCalls.Add(1)
					_, err := io.Copy(io.Discard, r.Body)
					assert.NoError(t, err)
					w.Header().Set("Trailer", "X-Result-Trailer")
					_, _ = w.Write([]byte("upstream"))
					w.Header().Set("X-Result-Trailer", "done")
				}))
				defer backend.Close()
				first := func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
					assert.Equal(t, "value", r.Context.Parameters["configured"])
					if eventPhase(r) != phase {
						return ok(&pb.ContinueResponse{}), nil
					}
					denied.Store(true)
					return &pb.InterceptResponse{Result: &pb.InterceptResponse_ImmediateResponse{ImmediateResponse: &pb.ImmediateResponse{StatusCode: 403, Body: []byte("stop:" + phase), Headers: []*pb.Header{{Key: "X-Denied", Value: phase}}}}}, nil
				}
				second := func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
					if denied.Load() {
						afterDenial.Add(1)
					}
					return ok(&pb.ContinueResponse{}), nil
				}
				s := interceptedGateway(t, backend.URL, allProcessing, "", []string{transport, transport}, first, second)
				req, err := http.NewRequest("POST", s.URL, strings.NewReader("upload"))
				require.NoError(t, err)
				req.ContentLength = -1
				req.Trailer = http.Header{"X-Upload-Trailer": []string{"done"}}
				response, err := s.Client().Do(req)
				require.NoError(t, err)
				defer func() { _ = response.Body.Close() }()
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				require.True(t, denied.Load())
				require.Equal(t, 403, response.StatusCode)
				require.Equal(t, "stop:"+phase, string(body))
				require.Equal(t, phase, response.Header.Get("X-Denied"))
				require.Zero(t, afterDenial.Load())
				want := int32(0)
				if strings.HasPrefix(phase, "response") {
					want = 1
				}
				require.Equal(t, want, backendCalls.Load())
			})
		}
	}
}

func TestRESTBodyAndResponseChainSeesMutations(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "request\x00", string(body))
		assert.Empty(t, r.Trailer)
		w.Header().Set("Trailer", "X-Download")
		_, _ = w.Write([]byte("original"))
		w.Header().Set("X-Download", "original")
	}))
	defer backend.Close()
	first := func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		switch eventPhase(r) {
		case config.RequestBody:
			return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("request\x00")}}), nil
		case config.RequestTrailers:
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Upload", "changed")}), nil
		case config.ResponseHeaders:
			code := int32(201)
			return ok(&pb.ContinueResponse{StatusCode: &code, HeaderMutation: set("X-Response", "changed")}), nil
		case config.ResponseBody:
			return ok(&pb.ContinueResponse{BodyMutation: &pb.BodyMutation{Data: []byte("response\x00")}}), nil
		case config.ResponseTrailers:
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Download", "changed")}), nil
		default:
			return ok(&pb.ContinueResponse{}), nil
		}
	}
	second := func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		switch eventPhase(r) {
		case config.RequestBody:
			assert.Equal(t, []byte("request\x00"), r.GetRequestBody().Data)
		case config.RequestTrailers:
			assert.Equal(t, "changed", getHeader(r.GetRequestTrailers().Trailers, "X-Upload"))
		case config.ResponseHeaders:
			assert.EqualValues(t, 201, *r.GetResponseHeaders().StatusCode)
			assert.Equal(t, "changed", getHeader(r.GetResponseHeaders().Headers, "X-Response"))
		case config.ResponseBody:
			assert.Equal(t, []byte("response\x00"), r.GetResponseBody().Data)
		case config.ResponseTrailers:
			assert.Equal(t, "changed", getHeader(r.GetResponseTrailers().Trailers, "X-Download"))
		}
		return ok(&pb.ContinueResponse{}), nil
	}
	// Body replacement clears obsolete trailers, so this test checks body and header
	// propagation. Separate trailer tests exercise unchanged bodies through all phases.
	s := interceptedGateway(t, backend.URL, allProcessing, "", []string{restTransport, grpcTransport}, first, second)
	status, h, body := request(t, s, "POST", "/", "original", nil)
	require.Equal(t, 201, status)
	require.Equal(t, "response\x00", body)
	require.Equal(t, "changed", h.Get("X-Response"))
}

func TestRESTTrailersReachNextProcessorAndPeers(t *testing.T) {
	var phases atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, err := io.Copy(io.Discard, r.Body)
		assert.NoError(t, err)
		assert.Equal(t, "changed", r.Trailer.Get("X-Upload"))
		w.Header().Set("Trailer", "X-Download")
		_, _ = w.Write([]byte("body"))
		w.Header().Set("X-Download", "old")
	}))
	defer backend.Close()
	first := func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		if r.GetRequestTrailers() != nil {
			assert.Equal(t, "old", getHeader(r.GetRequestTrailers().Trailers, "X-Upload"))
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Upload", "changed")}), nil
		}
		if r.GetResponseTrailers() != nil {
			return ok(&pb.ContinueResponse{TrailerMutation: set("X-Download", "changed")}), nil
		}
		return ok(&pb.ContinueResponse{}), nil
	}
	second := func(_ context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		if event := r.GetRequestTrailers(); event != nil {
			assert.Equal(t, "changed", getHeader(event.Trailers, "X-Upload"))
			phases.Add(1)
		}
		if event := r.GetResponseTrailers(); event != nil {
			assert.Equal(t, "changed", getHeader(event.Trailers, "X-Download"))
			phases.Add(1)
		}
		return ok(&pb.ContinueResponse{}), nil
	}
	s := interceptedGateway(t, backend.URL, allProcessing, "", []string{restTransport, restTransport}, first, second)
	req, err := http.NewRequest("POST", s.URL, strings.NewReader("body"))
	require.NoError(t, err)
	req.ContentLength = -1
	req.Trailer = http.Header{"X-Upload": []string{"old"}}
	response, err := s.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	_, err = io.Copy(io.Discard, response.Body)
	require.NoError(t, err)
	require.Equal(t, "changed", response.Trailer.Get("X-Download"))
	require.EqualValues(t, 2, phases.Load(), "both trailer events expected")
}
