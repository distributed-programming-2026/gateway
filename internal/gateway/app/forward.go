package app

import (
	"errors"
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func (g *Gateway) forward(w http.ResponseWriter, x *exchange) {
	req := x.request
	g.proxy.Forward(w, req, x.route, func(response *http.Response) error {
		original := x.request
		x.request = x.request.WithContext(response.Request.Context())
		err := g.processResponse(x, response)
		x.request = original
		return err
	}, func(err error) {
		if req.Context().Err() != nil {
			return
		}
		statusCode := 502
		code := "upstream_unavailable"
		if timeout(err) {
			statusCode = 504
			code = "upstream_timeout"
		}
		var pe *processingError
		if errors.As(err, &pe) {
			statusCode = pe.status
			code = pe.code
			x.failed[pe.interceptor] = true
		}
		if pe != nil {
			x.gatewayError = &pb.GatewayError{Code: code, InterceptorId: pe.interceptor}
			response := localResponse(req, statusCode, http.StatusText(statusCode))
			defer func() { _ = response.Body.Close() }()
			g.write(w, x, response)
			return
		}
		g.local(w, x, failure(code, statusCode))
	})
}
