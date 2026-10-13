package app

import (
	"fmt"
	"net/http"
	"sync/atomic"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
)

type Proxy interface {
	Forward(http.ResponseWriter, *http.Request, *config.Route, func(*http.Response) error, func(error))
}
type Gateway struct {
	conf     *config.Config
	clients  map[string]Interceptor
	proxy    Proxy
	sequence atomic.Uint64
}

func New(conf *config.Config, clients map[string]Interceptor, proxy Proxy) *Gateway {
	return &Gateway{conf: conf, clients: clients, proxy: proxy}
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, original *http.Request) {
	req := original.Clone(original.Context())
	// The server fills this map when the original body reaches EOF.
	// Preserve it through proxy request cloning for streaming uploads.
	req.Trailer = original.Trailer
	x := &exchange{request: req, id: fmt.Sprintf("gw-%d", g.sequence.Add(1)), failed: map[string]bool{}}
	if err := g.selectRoute(x); err != nil {
		if x.route == nil {
			http.NotFound(w, req)
		} else {
			g.local(w, x, err)
		}
		return
	}
	for _, h := range g.conf.Gateway.InternalHeaders.Headers {
		req.Header.Del(h)
	}
	if x.route.BasicAuth != nil && !authorized(req, x.route.BasicAuth) {
		response := localResponse(req, 401, "Unauthorized")
		response.Header.Set("WWW-Authenticate", `Basic realm="gateway"`)
		x.gatewayError = &pb.GatewayError{Code: "basic_auth_denied"}
		g.finishLocal(w, x, response)
		return
	}
	if g.requestPhase(w, x, config.RequestHeaders, nil) {
		return
	}
	if req.Header.Get("Upgrade") != "" && (g.buffered(x.route, false) || g.buffered(x.route, true)) {
		g.local(w, x, failure("upgrade_processing_unsupported", 502))
		return
	}
	if g.processRequestBody(w, x) {
		return
	}
	if r := x.route.DirectResponse; r != nil {
		response := localResponse(req, r.Status, r.InlineString)
		defer func() { _ = response.Body.Close() }()
		g.finishLocal(w, x, response)
		return
	}
	if x.route.URL == nil {
		http.NotFound(w, req)
		return
	}
	g.forward(w, x)
}
