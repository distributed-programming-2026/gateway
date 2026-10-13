// Package server composes the proxy, phase clients and process lifecycle.
package server

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/distributed-programming-2026/gateway/internal/gateway/app"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/interceptors"
	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/proxy"
)

type Runtime struct {
	Handler      http.Handler
	Health       http.Handler
	ready        atomic.Bool
	closeClients func() error
	proxy        *proxy.Proxy
}

func New(c *config.Config) (*Runtime, error) {
	clients, closeClients, err := interceptors.New(c.Interceptors)
	if err != nil {
		return nil, err
	}
	p := proxy.New(c.Gateway)
	r := &Runtime{Handler: app.New(c, clients, p), closeClients: closeClients, proxy: p}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(204) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if r.ready.Load() {
			w.WriteHeader(204)
		} else {
			w.WriteHeader(503)
		}
	})
	r.Health = mux
	return r, nil
}
func (r *Runtime) Close() error { r.ready.Store(false); r.proxy.Close(); return r.closeClients() }

// Serve takes already-bound listeners, so readiness never races port binding.
func (r *Runtime) Serve(ctx context.Context, proxyListener net.Listener, healthListeners ...net.Listener) error {
	listeners := append([]net.Listener{proxyListener}, healthListeners...)
	servers := make([]*http.Server, len(listeners))
	done := make(chan error, len(listeners))
	for i, l := range listeners {
		handler := r.Health
		if i == 0 {
			handler = r.Handler
		}
		srv := &http.Server{Handler: handler, Protocols: Protocols(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 10 * time.Minute, BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) }}
		servers[i] = srv
		go func() { done <- srv.Serve(l) }()
	}
	r.ready.Store(true)
	var result error
	received := 0
	select {
	case <-ctx.Done():
	case err := <-done:
		received++
		if !errors.Is(err, http.ErrServerClosed) {
			result = err
		}
	}
	r.ready.Store(false)
	result = errors.Join(result, shutdown(servers, 30*time.Second))
	for received < len(servers) {
		err := <-done
		received++
		if !errors.Is(err, http.ErrServerClosed) {
			result = errors.Join(result, err)
		}
	}
	return result
}

// Protocols uses native Go HTTP/1 and cleartext HTTP/2 support.
func Protocols() *http.Protocols {
	p := new(http.Protocols)
	p.SetHTTP1(true)
	p.SetUnencryptedHTTP2(true)
	return p
}
