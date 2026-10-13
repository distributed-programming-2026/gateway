package integration_tests

import (
	"bufio"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSEFlushesBeforeUpstreamFinishes(t *testing.T) {
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: first\n\n"))
		w.(http.Flusher).Flush()
		<-release
		_, _ = w.Write([]byte("data: second\n\n"))
	}))
	defer backend.Close()
	defer unblock()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s', timeout: 0s, idleTimeout: 0s}]\n", backend.URL))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", s.URL, http.NoBody)
	require.NoError(t, err)
	response, err := s.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	reader := bufio.NewReader(response.Body)
	first, err := reader.ReadString('\n')
	require.NoError(t, err)
	require.Equal(t, "data: first\n", first)
	unblock()
	rest, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "\ndata: second\n\n", string(rest))
}
func TestUpgradeProxiesBidirectionalBytes(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "echo", r.Header.Get("Upgrade"))
		conn, rw, err := w.(http.Hijacker).Hijack()
		if !assert.NoError(t, err) {
			return
		}
		defer func() { _ = conn.Close() }()
		_, err = rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
		if !assert.NoError(t, err) {
			return
		}
		if !assert.NoError(t, rw.Flush()) {
			return
		}
		_, _ = io.Copy(conn, rw)
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s', timeout: 0s}]\n", backend.URL))
	conn, err := net.DialTimeout("tcp", s.Listener.Addr().String(), time.Second)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()
	require.NoError(t, conn.SetDeadline(time.Now().Add(2*time.Second)))
	_, err = fmt.Fprintf(conn, "GET / HTTP/1.1\r\nHost: example\r\nConnection: Upgrade\r\nUpgrade: echo\r\n\r\n")
	require.NoError(t, err)
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, &http.Request{Method: "GET"})
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, 101, response.StatusCode)
	for _, data := range []string{"client-to-upstream\x00", "second-message"} {
		_, err = io.WriteString(conn, data)
		require.NoError(t, err)
		reply := make([]byte, len(data))
		_, err = io.ReadFull(reader, reply)
		require.NoError(t, err)
		require.Equal(t, data, string(reply))
	}
}
func TestGzipAndForwardedHeadersPreserved(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "gzip", r.Header.Get("Accept-Encoding"))
		assert.NotContains(t, r.Header.Get("X-Forwarded-For"), "spoofed")
		assert.Equal(t, "http", r.Header.Get("X-Forwarded-Proto"))
		assert.Equal(t, tenantHost, r.Header.Get("X-Forwarded-Host"))
		assert.Equal(t, strings.TrimPrefix(r.Header.Get("X-Upstream"), "http://"), r.Host)
		w.Header().Set("Content-Encoding", "gzip")
		writer := gzip.NewWriter(w)
		_, err := writer.Write([]byte("compressed\x00"))
		assert.NoError(t, err)
		assert.NoError(t, writer.Close())
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s'}]\n", backend.URL))
	req, err := http.NewRequest("GET", s.URL, http.NoBody)
	require.NoError(t, err)
	req.Host = tenantHost
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-Upstream", backend.URL)
	req.Header.Set("X-Forwarded-For", "spoofed")
	response, err := s.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, "gzip", response.Header.Get("Content-Encoding"))
	reader, err := gzip.NewReader(response.Body)
	require.NoError(t, err)
	defer func() { _ = reader.Close() }()
	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.Equal(t, "compressed\x00", string(body))
}
