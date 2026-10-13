package integration_tests

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoutingRewriteAndProxy(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		assert.Equal(t, "/items/42?x=1&x=2", r.URL.RequestURI(),
			"wrong upstream request: %s %s %q", r.Method, r.URL, body)
		assert.Equal(t, "POST", r.Method,
			"wrong upstream request: %s %s %q", r.Method, r.URL, body)
		assert.Equal(t, "binary\x00body", string(body),
			"wrong upstream request: %s %s %q", r.Method, r.URL, body)
		assert.EqualValues(t, 2, len(r.Header.Values(multiHeader)),
			"lost repeated headers")
		assert.Equal(t, strings.TrimPrefix(r.Host, "http://"), r.Host,
			"invalid Host")

		w.Header().Add("Set-Cookie", "a=1")
		w.Header().Add("Set-Cookie", "b=2")
		w.WriteHeader(201)
		_, _ = w.Write([]byte("upstream"))
	}))
	defer upstream.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: backend
weight: 100
routes:
  - pattern: '^/old/([^/]+)$'
    internalRedirect:
      old: '^/old/([^/]+)$'
      new: '/api/items/$1'
  - pattern: '^/api/items/[^/]+$'
    rewrite:
      old: '^/api/(.*)$'
      new: '/$1'
    target: %s
    addHeaders:
      X-Item:
        pathParametersPattern: '^/items/{id}$'
        value: 'item={id}'
---
kind: gateway/service/v1
name: fallback
weight: -100
routes:
  - pattern: '^/.*$'
    directResponse: {status: 404, inlineString: missing}
`, upstream.URL))
	status, h, body := request(t, s, "POST", "/old/42?x=1&x=2", "binary\x00body", http.Header{multiHeader: []string{"one", "two"}})
	require.EqualValues(t, 201, status,
		"unexpected response %d %v %q", status, h, body)
	require.Equal(t, "upstream", body,
		"unexpected response %d %v %q", status, h, body)
	require.Equal(t, "item=42", h.Get("X-Item"),
		"unexpected response %d %v %q", status, h, body)
	require.EqualValues(t, 2, len(h.Values("Set-Cookie")),
		"unexpected response %d %v %q", status, h, body)

	status, _, body = request(t, s, "GET", "/missing", "", nil)
	require.EqualValues(t, 404, status,
		"fallback failed")
	require.Equal(t, "missing", body,
		"fallback failed")
}
