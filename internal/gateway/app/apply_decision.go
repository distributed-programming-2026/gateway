package app

import (
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"google.golang.org/protobuf/proto"
)

func applyDecision(x *exchange, phase string, response *http.Response, body []byte, decision *pb.InterceptResponse) []byte {
	req := x.request
	result := decision.GetContinueResponse()
	current := req.Header
	if response != nil {
		current = response.Header
	}
	applyHeaders(current, result.HeaderMutation)
	if result.RequestMutation != nil {
		m := result.RequestMutation
		if m.Method != nil {
			req.Method = *m.Method
		}
		if m.Authority != nil {
			req.Host = *m.Authority
		}
		if m.Path != nil {
			setPath(req, *m.Path)
		}
		if m.RawQuery != nil {
			req.URL.RawQuery = *m.RawQuery
		}
	}
	if result.StatusCode != nil {
		response.StatusCode = int(*result.StatusCode)
	}
	if result.ResponseHeaderMutation != nil {
		x.pending = append(x.pending, proto.Clone(result.ResponseHeaderMutation).(*pb.HeaderMutation))
	}
	if result.TrailerMutation != nil {
		trailer := req.Trailer
		if response != nil {
			trailer = response.Trailer
		}
		applyHeaders(trailer, result.TrailerMutation)
	}
	if result.BodyMutation == nil {
		return body
	}
	replacement := result.BodyMutation.Data
	applyBody(x, phase, response, replacement)
	return replacement
}
func applyBody(x *exchange, phase string, response *http.Response, body []byte) {
	if response == nil {
		replaceRequestBody(x.request, body, true)
		x.requestReplaced = x.requestReplaced || phase == config.RequestHeaders
		return
	}
	replaceResponseBody(response, body)
	x.responseReplaced = x.responseReplaced || phase == config.ResponseHeaders
}

func immediateResponse(x *exchange, immediate *pb.ImmediateResponse) *http.Response {
	result := localResponse(x.request, int(immediate.StatusCode), string(immediate.Body))
	for _, m := range x.pending {
		applyHeaders(result.Header, m)
	}
	h := make(http.Header)
	for _, v := range immediate.Headers {
		h.Add(v.Key, v.Value)
	}
	for k, vs := range h {
		result.Header[k] = vs
	}
	return result
}
