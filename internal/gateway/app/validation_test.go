package app

import (
	"testing"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/stretchr/testify/require"
)

func TestMutationPhaseContract(t *testing.T) {
	mutations := []struct {
		name    string
		value   *pb.ContinueResponse
		allowed []string
	}{
		{"header", &pb.ContinueResponse{HeaderMutation: &pb.HeaderMutation{}}, []string{config.RequestHeaders, config.ResponseHeaders}},
		{"request", &pb.ContinueResponse{RequestMutation: &pb.RequestMutation{}}, []string{config.RequestHeaders}},
		{"status", &pb.ContinueResponse{StatusCode: pointer(int32(201))}, []string{config.ResponseHeaders}},
		{"pending-response-header", &pb.ContinueResponse{ResponseHeaderMutation: &pb.HeaderMutation{}}, []string{config.RequestHeaders, config.RequestBody, config.RequestTrailers}},
		{"body", &pb.ContinueResponse{BodyMutation: &pb.BodyMutation{}}, []string{config.RequestHeaders, config.ResponseHeaders, config.RequestBody, config.ResponseBody}},
		{"trailer", &pb.ContinueResponse{TrailerMutation: &pb.HeaderMutation{}}, []string{config.RequestTrailers, config.ResponseTrailers}},
	}
	phases := []string{config.RequestHeaders, config.RequestBody, config.RequestTrailers, config.ResponseHeaders, config.ResponseBody, config.ResponseTrailers}
	for _, mutation := range mutations {
		for _, phase := range phases {
			t.Run(mutation.name+"/"+phase, func(t *testing.T) {
				decision := &pb.InterceptResponse{Result: &pb.InterceptResponse_ContinueResponse{ContinueResponse: mutation.value}}
				mode := config.ProcessingMode{RequestBody: config.BodyBuffered, ResponseBody: config.BodyBuffered}
				err := validateDecision(decision, phase, &config.Route{}, mode)
				allowed := false
				for _, p := range mutation.allowed {
					allowed = allowed || p == phase
				}
				if allowed {
					require.NoError(t, err)
				} else {
					require.ErrorContains(t, err, "wrong phase")
				}
			})
		}
	}
}
func TestInvalidDecisionsAndRequestMetadata(t *testing.T) {
	for _, decision := range []*pb.InterceptResponse{nil, {}, {Result: &pb.InterceptResponse_ContinueResponse{}}, {Result: &pb.InterceptResponse_ImmediateResponse{}}} {
		require.Error(t, validateDecision(decision, config.RequestHeaders, &config.Route{}, config.ProcessingMode{}))
	}
	for _, phase := range []string{config.RequestBody, config.ResponseBody} {
		decision := &pb.InterceptResponse{Result: &pb.InterceptResponse_ContinueResponse{ContinueResponse: &pb.ContinueResponse{BodyMutation: &pb.BodyMutation{}}}}
		require.ErrorContains(t, validateDecision(decision, phase, &config.Route{}, config.ProcessingMode{}), "requires buffer")
		require.ErrorContains(t, validateDecision(decision, phase, &config.Route{UseHTTP2: true}, config.ProcessingMode{}), "HTTP2")
	}
	for _, status := range []int32{199, 600} {
		require.Error(t, validateImmediate(&pb.ImmediateResponse{StatusCode: status}))
		require.Error(t, validateDecision(&pb.InterceptResponse{Result: &pb.InterceptResponse_ContinueResponse{ContinueResponse: &pb.ContinueResponse{StatusCode: &status}}}, config.ResponseHeaders, &config.Route{}, config.ProcessingMode{}))
	}
	for _, m := range []*pb.RequestMutation{
		{Method: pointer("bad method")}, {Path: pointer("relative")}, {Path: pointer("/path?query")},
		{Authority: pointer("user@host")}, {Authority: pointer("host/path")}, {Authority: pointer("host#fragment")},
		{RawQuery: pointer("?query")}, {RawQuery: pointer("line\nbreak")},
	} {
		require.Error(t, validateRequestMutation(m))
	}
	require.NoError(t, validateRequestMutation(&pb.RequestMutation{Method: pointer("PATCH"), Path: pointer("/path"), Authority: pointer("[::1]:80"), RawQuery: pointer("a=1&a=2")}))
	for _, m := range []*pb.HeaderMutation{
		{Set: []*pb.HeaderValues{nil}}, {Append: []*pb.HeaderValues{{Key: "X-Test", Values: []string{"a"}}, {Key: "x-test", Values: []string{"b"}}}},
		{Remove: []string{"Authorization"}}, {Set: []*pb.HeaderValues{{Key: "X-Test", Values: []string{"\n"}}}},
	} {
		require.Error(t, validateMutation(m, true))
	}
	require.Error(t, validateImmediate(&pb.ImmediateResponse{StatusCode: 403, Headers: []*pb.Header{nil}}))
}
func pointer[T any](v T) *T { return &v }
