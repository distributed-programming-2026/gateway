package app

import (
	"errors"
	"io"
	"net/http"
	"strings"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"golang.org/x/net/http/httpguts"
)

func (g *Gateway) local(w http.ResponseWriter, x *exchange, err error) {
	if x.request.Context().Err() != nil {
		return
	}
	pe := failure(interceptorError, 502)
	var found *processingError
	if errors.As(err, &found) {
		pe = found
	}
	x.gatewayError = &pb.GatewayError{Code: pe.code, InterceptorId: pe.interceptor}
	if pe.interceptor != "" {
		x.failed[pe.interceptor] = true
	}
	response := localResponse(x.request, pe.status, http.StatusText(pe.status))
	defer func() { _ = response.Body.Close() }()
	g.finishLocal(w, x, response)
}
func (g *Gateway) finishLocal(w http.ResponseWriter, x *exchange, response *http.Response) {
	if err := g.processResponse(x, response); err != nil {
		_ = response.Body.Close()
		pe := failure(interceptorError, 502)
		var found *processingError
		if errors.As(err, &found) {
			pe = found
		}
		response = localResponse(x.request, pe.status, http.StatusText(pe.status)) //nolint:bodyclose // write below takes ownership and closes the replacement body.
	}
	g.write(w, x, response)
}
func localResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{StatusCode: status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), ContentLength: int64(len(body)), Request: req, Trailer: make(http.Header)}
}
func (g *Gateway) addHeaders(x *exchange, response *http.Response) {
	for name, h := range x.route.AddHeaders {
		if !h.Always && (response.StatusCode < 200 || response.StatusCode >= 300) {
			continue
		}
		if value, ok := config.HeaderValue(h, x.request.URL.Path); ok && httpguts.ValidHeaderFieldValue(value) {
			response.Header.Set(name, value)
		}
	}
}
func (g *Gateway) write(w http.ResponseWriter, x *exchange, response *http.Response) {
	defer func() { _ = response.Body.Close() }()
	g.addHeaders(x, response)
	for k, values := range response.Header {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(response.StatusCode)
	if x.request.Method != http.MethodHead && response.StatusCode != 204 && response.StatusCode != 304 {
		_, _ = io.Copy(w, response.Body)
	}
}
func enforceBody(req *http.Request, response *http.Response) {
	if req.Method == http.MethodHead || response.StatusCode == 204 || response.StatusCode == 304 {
		_ = response.Body.Close()
		response.Body = http.NoBody
		if response.StatusCode == 204 {
			response.Header.Del("Content-Length")
			response.ContentLength = 0
		}
	}
}
