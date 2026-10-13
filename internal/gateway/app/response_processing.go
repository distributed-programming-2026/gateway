package app

import (
	"bytes" // SHA htpasswd compatibility, not password generation.
	"errors"
	"io"
	"net/http"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
)

func (g *Gateway) processResponse(x *exchange, response *http.Response) error {
	for _, m := range x.pending {
		applyHeaders(response.Header, m)
	}
	stopped, err := g.responsePhase(x, config.ResponseHeaders, response, nil)
	if err != nil {
		return err
	}
	if !stopped && !x.responseReplaced && g.buffered(x.route, true) {
		if err = g.processResponseBody(x, response); err != nil {
			return err
		}
	}
	g.addHeaders(x, response)
	enforceBody(x.request, response)
	return nil
}
func (g *Gateway) responsePhase(x *exchange, phase string, response *http.Response, body []byte) (bool, error) {
	immediate, err := g.phase(x, phase, response, body) //nolint:bodyclose // Ownership transfers to response; the proxy or write closes its body.
	if err != nil {
		return false, err
	}
	if immediate == nil {
		return false, nil
	}
	_ = response.Body.Close()
	*response = *immediate
	return true, nil
}
func (g *Gateway) processResponseBody(x *exchange, response *http.Response) error {
	body, err := readLimited(response.Body, g.bodyLimit(x.route, true))
	if err != nil {
		if timeout(err) {
			return failure("upstream_timeout", 504)
		}
		if errors.Is(err, errBodyLimit) {
			return failure("response_body_too_large", 502)
		}
		return failure("upstream_unavailable", 502)
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	stopped, err := g.responsePhase(x, config.ResponseBody, response, body)
	if err != nil || stopped {
		return err
	}
	if len(response.Trailer) > 0 {
		_, err = g.responsePhase(x, config.ResponseTrailers, response, nil)
	}
	return err
}
