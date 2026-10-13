package app

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
)

func (g *Gateway) buffered(route *config.Route, response bool) bool {
	for _, i := range route.Interceptors {
		if (!response && i.ProcessingMode.RequestBody == config.BodyBuffered) || (response && i.ProcessingMode.ResponseBody == config.BodyBuffered) {
			return true
		}
	}
	return false
}
func (g *Gateway) bodyLimit(route *config.Route, response bool) int64 {
	var limit int64
	for _, i := range route.Interceptors {
		m := i.ProcessingMode
		enabled := m.Enabled(config.RequestHeaders) || m.Enabled(config.RequestBody) || m.Enabled(config.RequestTrailers)
		l := *m.MaxRequestBodyBytes
		if response {
			enabled = enabled || m.Enabled(config.ResponseHeaders) || m.Enabled(config.ResponseBody) || m.Enabled(config.ResponseTrailers)
			l = *m.MaxResponseBodyBytes
		}
		if enabled && (limit == 0 || l < limit) {
			limit = l
		}
	}
	if limit == 0 {
		return 1 << 20
	}
	return limit
}

var errBodyLimit = errors.New("body limit exceeded")

func readLimited(body io.ReadCloser, limit int64) ([]byte, error) {
	if body == nil {
		return []byte{}, nil
	}
	defer func() { _ = body.Close() }()
	b, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errBodyLimit
	}
	return b, nil
}

// requestPhase writes an immediate response or error and reports whether processing stopped.
func (g *Gateway) requestPhase(w http.ResponseWriter, x *exchange, phase string, body []byte) bool {
	response, err := g.phase(x, phase, nil, body)
	if err != nil {
		g.local(w, x, err)
		return true
	}
	if response == nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	g.write(w, x, response)
	return true
}
func (g *Gateway) processRequestBody(w http.ResponseWriter, x *exchange) bool {
	if x.requestReplaced || !g.buffered(x.route, false) {
		return false
	}
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(time.Now().Add(60 * time.Second))
	body, err := readLimited(x.request.Body, g.bodyLimit(x.route, false))
	_ = controller.SetReadDeadline(time.Time{})
	if err != nil {
		g.local(w, x, requestReadError(err))
		return true
	}
	replaceRequestBody(x.request, body, false)
	if g.requestPhase(w, x, config.RequestBody, body) {
		return true
	}
	return len(x.request.Trailer) > 0 && g.requestPhase(w, x, config.RequestTrailers, nil)
}
func requestReadError(err error) error {
	if timeout(err) {
		return failure("request_body_read_timeout", 408)
	}
	if errors.Is(err, errBodyLimit) {
		return failure("request_body_too_large", 413)
	}
	return failure("request_body_read_error", 400)
}
