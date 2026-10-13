package integration_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestUpstreamTimeoutsAndUnavailable(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/idle" {
			time.Sleep(150 * time.Millisecond)
			_, _ = w.Write([]byte("late"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		f := w.(http.Flusher)
		for i := 0; i < 10; i++ {
			_, _ = w.Write([]byte("tick\n"))
			f.Flush()
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: backend
routes:
  - pattern: '^/idle$'
    target: '%s'
    timeout: 0s
    idleTimeout: 20ms
  - pattern: '^/stream$'
    target: '%s'
    timeout: 0s
    idleTimeout: 40ms
  - pattern: '^/unavailable$'
    target: http://127.0.0.1:1
`, backend.URL, backend.URL))
	status, _, _ := request(t, s, "GET", "/idle", "", nil)
	require.EqualValues(t, 504, status,
		"idle status %d", status)

	status, _, body := request(t, s, "GET", "/stream", "", nil)
	require.EqualValues(t, 200, status,
		"active stream timed out")
	require.EqualValues(t, 10, strings.Count(body, "tick"),
		"active stream timed out")

	status, _, _ = request(t, s, "GET", "/unavailable", "", nil)
	require.EqualValues(t, 502, status,
		"unavailable status")
}
