// Package proxy owns HTTP/1, HTTP/2 and streaming transport details.
package proxy

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"
)

// Idle is request-scoped, not an idle keep-alive timeout. It starts only once
// the upload is complete and is reset by upstream response activity.
type idleTransport struct {
	base http.RoundTripper
	idle time.Duration
}
type activity struct {
	mu      sync.Mutex
	timer   *time.Timer
	cancel  context.CancelFunc
	idle    time.Duration
	closed  bool
	expired bool
}

func (a *activity) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed || a.expired {
		return
	}
	if a.timer == nil {
		a.timer = time.AfterFunc(a.idle, func() {
			a.mu.Lock()
			if !a.closed {
				a.expired = true
				a.cancel()
			}
			a.mu.Unlock()
		})
	} else {
		a.timer.Reset(a.idle)
	}
}
func (a *activity) close() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.timer != nil {
		a.timer.Stop()
	}
	a.cancel()
}
func (a *activity) timedout() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.expired }
func (t *idleTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.idle == 0 {
		return t.base.RoundTrip(req)
	}
	ctx, cancel := context.WithCancel(req.Context())
	a := &activity{cancel: cancel, idle: t.idle}
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			a.reset()
		}
	}, GotFirstResponseByte: a.reset}
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	response, err := t.base.RoundTrip(req)
	if err != nil {
		expired := a.timedout()
		a.close()
		if expired {
			return nil, context.DeadlineExceeded
		}
		return nil, err
	}
	a.reset()
	if response.StatusCode == http.StatusSwitchingProtocols {
		a.closeTimer()
		return response, nil
	}
	response.Body = &activityBody{ReadCloser: response.Body, a: a}
	return response, nil
}
func (a *activity) closeTimer() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.closed = true
	if a.timer != nil {
		a.timer.Stop()
	}
}

type activityBody struct {
	io.ReadCloser
	a *activity
}

func (b *activityBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if n > 0 {
		b.a.reset()
	}
	if b.a.timedout() {
		return n, context.DeadlineExceeded
	}
	if errors.Is(err, io.EOF) {
		b.a.closeTimer()
	}
	return n, err
}
func (b *activityBody) Close() error { err := b.ReadCloser.Close(); b.a.close(); return err }
