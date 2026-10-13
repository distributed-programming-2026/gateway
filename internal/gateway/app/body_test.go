package app

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/stretchr/testify/require"
)

type brokenBody struct {
	err    error
	closed bool
}

func (b *brokenBody) Read([]byte) (int, error) { return 0, b.err }
func (b *brokenBody) Close() error             { b.closed = true; return nil }
func TestBodyReadErrorsAndClosure(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{io.ErrUnexpectedEOF, "request_body_read_error", 400},
		{context.DeadlineExceeded, "request_body_read_timeout", 408},
		{errBodyLimit, "request_body_too_large", 413},
	} {
		body := &brokenBody{err: tc.err}
		_, err := readLimited(body, 10)
		require.ErrorIs(t, err, tc.err)
		require.True(t, body.closed)
		var pe *processingError
		require.True(t, errors.As(requestReadError(err), &pe))
		require.Equal(t, tc.code, pe.code)
		require.Equal(t, tc.status, pe.status)
	}
	data, err := readLimited(nil, 10)
	require.NoError(t, err)
	require.Empty(t, data)
}
func TestHTTPSContextAndEndOfStream(t *testing.T) {
	req := httptest.NewRequest("GET", "https://tenant.example/path?a=1", http.NoBody)
	req.URL.Scheme = ""
	event := processingEvent(&exchange{request: req}, config.Invocation{}, config.RequestHeaders, nil, nil)
	require.Equal(t, "https", event.Context.Request.Scheme)
	require.True(t, event.GetRequestHeaders().EndOfStream)
}
