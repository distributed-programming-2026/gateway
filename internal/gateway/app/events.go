package app

import (
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
)

func headers(h http.Header) []*pb.Header {
	result := make([]*pb.Header, 0, len(h))
	for k, values := range h {
		for _, v := range values {
			result = append(result, &pb.Header{Key: k, Value: v})
		}
	}
	return result
}

func processingEvent(x *exchange, invocation config.Invocation, phase string, response *http.Response, body []byte) *pb.InterceptRequest {
	req := x.request
	request := &pb.InterceptRequest{Context: &pb.ProcessingContext{RequestId: x.id, Request: &pb.HttpRequest{Method: req.Method, Scheme: req.URL.Scheme, Authority: req.Host, Path: req.URL.Path, RawQuery: req.URL.RawQuery, Headers: headers(req.Header)}, PathParameters: config.Parameters(invocation.RE, req.URL.Path), Parameters: invocation.Parameters, GatewayError: x.gatewayError}}
	if request.Context.Request.Scheme == "" {
		request.Context.Request.Scheme = "http"
		if req.TLS != nil {
			request.Context.Request.Scheme = "https"
		}
	}
	switch phase {
	case config.RequestHeaders:
		request.Event = &pb.InterceptRequest_RequestHeaders{RequestHeaders: &pb.HttpHeaders{Headers: headers(req.Header), EndOfStream: (req.Body == nil || req.Body == http.NoBody || req.ContentLength == 0) && len(req.Trailer) == 0}}
	case config.ResponseHeaders:
		statusCode := int32(response.StatusCode) // #nosec G115 -- Upstream HTTP status codes have three digits; local statuses are validated.
		request.Event = &pb.InterceptRequest_ResponseHeaders{ResponseHeaders: &pb.HttpHeaders{Headers: headers(response.Header), StatusCode: &statusCode, EndOfStream: response.ContentLength == 0 && len(response.Trailer) == 0}}
	case config.RequestBody:
		request.Event = &pb.InterceptRequest_RequestBody{RequestBody: &pb.HttpBody{Data: body, EndOfStream: len(req.Trailer) == 0}}
	case config.ResponseBody:
		request.Event = &pb.InterceptRequest_ResponseBody{ResponseBody: &pb.HttpBody{Data: body, EndOfStream: len(response.Trailer) == 0}}
	case config.RequestTrailers:
		request.Event = &pb.InterceptRequest_RequestTrailers{RequestTrailers: &pb.HttpTrailers{Trailers: headers(req.Trailer)}}
	case config.ResponseTrailers:
		request.Event = &pb.InterceptRequest_ResponseTrailers{ResponseTrailers: &pb.HttpTrailers{Trailers: headers(response.Trailer)}}
	}

	return request
}
