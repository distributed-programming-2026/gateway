package proxy

import (
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifiedTLSUpstreamsHTTP1AndHTTP2(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "http1", true: "http2"}[http2], func(t *testing.T) {
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				want := 1
				if http2 {
					want = 2
				}
				assert.Equal(t, want, r.ProtoMajor)
				assert.Equal(t, r.URL.RawQuery, "a=1&a=2")
				w.Header().Set("Trailer", "X-TLS-Trailer")
				_, _ = w.Write([]byte("encrypted"))
				w.Header().Set("X-TLS-Trailer", "preserved")
			}))
			upstream.EnableHTTP2 = http2
			upstream.StartTLS()
			defer upstream.Close()
			roots := x509.NewCertPool()
			roots.AddCert(upstream.Certificate())
			p := New(config.Gateway{})
			defer p.Close()
			p.http1.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			p.h2tls.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
			target, err := url.Parse(upstream.URL)
			require.NoError(t, err)
			route := &config.Route{URL: target, UseHTTP2: http2}
			gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p.Forward(w, r, route, func(*http.Response) error { return nil }, func(err error) { assert.NoError(t, err); w.WriteHeader(502) })
			}))
			defer gateway.Close()
			response, err := gateway.Client().Get(gateway.URL + "/tls?a=1&a=2")
			require.NoError(t, err)
			defer func() { _ = response.Body.Close() }()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, 200, response.StatusCode)
			require.Equal(t, "encrypted", string(body))
			require.Equal(t, "preserved", response.Trailer.Get("X-TLS-Trailer"))
		})
	}
}
