package proxy

import (
	"errors"
	"io"
	"net/http"
)

// The HTTP transport can drain a closed body asynchronously and update its
// response's trailers. Keep that response separate from processor mutations.
type responseTransport struct{ base http.RoundTripper }

func (t responseTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	upstream, err := t.base.RoundTrip(req)
	if err != nil {
		return nil, err
	}
	if upstream.StatusCode == http.StatusSwitchingProtocols {
		return upstream, nil
	}
	response := new(http.Response)
	*response = *upstream
	response.Header = upstream.Header.Clone()
	response.Trailer = upstream.Trailer.Clone()
	// HTTP/2 permits Content-Length with trailers. HTTP/1 requires chunking
	// to deliver those trailers, so leave framing to the downstream server.
	if len(response.Trailer) > 0 {
		response.Header.Del("Content-Length")
		response.ContentLength = -1
	}
	response.Body = &trailerBody{ReadCloser: upstream.Body, upstream: upstream, response: response}
	return response, nil
}

type trailerBody struct {
	io.ReadCloser
	upstream *http.Response
	response *http.Response
}

func (b *trailerBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if errors.Is(err, io.EOF) {
		b.response.Trailer = b.upstream.Trailer.Clone()
	}
	return n, err
}
