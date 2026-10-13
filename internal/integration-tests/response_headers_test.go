package integration_tests

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResponseOnlyErrorPages(t *testing.T) {
	var calls atomic.Int32
	pages := restServer(t, func(input map[string]any) any {
		calls.Add(1)
		assert.Equal(t, config.ResponseHeaders, input["eventType"],
			"unexpected phase")

		event := input["event"].(map[string]any)
		assert.Equal(t, float64(503), event["statusCode"],
			"missing upstream status")

		return map[string]any{"responseType": "continueResponse", "headerMutation": map[string]any{jsonSet: []any{map[string]any{jsonHeaderKey: "Content-Type", jsonHeaderValues: []string{"text/html"}}}}, "bodyMutation": map[string]any{"data": base64.StdEncoding.EncodeToString([]byte("<h1>Unavailable</h1>"))}}
	})
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(503)
		_, _ = w.Write([]byte("internal details"))
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: pages
interceptor: {address: '%s', transport: rest}
---
kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/'
    target: '%s'
    interceptors:
      - id: pages
        processingMode: {requestHeaders: false, responseHeaders: true}
`, pages.URL, backend.URL))
	status, h, b := request(t, s, "GET", "/", "", nil)
	require.EqualValues(t, 503, status,
		"error page replacement failed")
	require.Equal(t, "<h1>Unavailable</h1>", b,
		"error page replacement failed")
	require.Equal(t, "text/html", h.Get("Content-Type"),
		"error page replacement failed")
	require.EqualValues(t, 1, calls.Load(),
		"error page replacement failed")
}
