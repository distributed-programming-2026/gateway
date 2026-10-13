package app

import (
	"errors"
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
)

func (g *Gateway) phase(x *exchange, phase string, response *http.Response, body []byte) (*http.Response, error) {
	for _, invocation := range x.route.Interceptors {
		if !invocation.ProcessingMode.Enabled(phase) || x.failed[invocation.ID] {
			continue
		}
		event := processingEvent(x, invocation, phase, response, body)
		decision, err := g.callInterceptor(x, invocation, phase, response != nil, event)
		if err != nil {
			x.failed[invocation.ID] = true
			return nil, err
		}
		if immediate := decision.GetImmediateResponse(); immediate != nil {
			return immediateResponse(x, immediate), nil
		}
		body = applyDecision(x, phase, response, body, decision)
	}
	return nil, nil
}
func (g *Gateway) callInterceptor(x *exchange, invocation config.Invocation, phase string, response bool, event *pb.InterceptRequest) (*pb.InterceptResponse, error) {
	client := g.clients[invocation.ID]
	var decision *pb.InterceptResponse
	var err error
	if client == nil {
		err = errors.New("missing interceptor client")
	} else {
		decision, err = client.Intercept(x.request.Context(), event)
	}
	if err == nil {
		err = validateDecision(decision, phase, x.route, invocation.ProcessingMode)
	}
	if err == nil {
		err = g.validateReplacement(x, decision, response)
	}
	if err != nil {
		code, statusCode := interceptorError, 502
		if timeout(err) {
			code, statusCode = "interceptor_timeout", 504
		}
		return nil, &processingError{code: code, status: statusCode, interceptor: invocation.ID}
	}
	return decision, nil
}
func (g *Gateway) validateReplacement(x *exchange, decision *pb.InterceptResponse, response bool) error {
	if immediate := decision.GetImmediateResponse(); immediate != nil && int64(len(immediate.Body)) > g.bodyLimit(x.route, true) {
		return errors.New("replacement too large")
	}
	result := decision.GetContinueResponse()
	if result == nil || result.BodyMutation == nil {
		return nil
	}
	if int64(len(result.BodyMutation.Data)) > g.bodyLimit(x.route, response) {
		return errors.New("replacement too large")
	}
	if x.request.Header.Get("Upgrade") != "" {
		return errors.New("upgrade body processing unsupported")
	}
	return nil
}
