package app

import (
	"bytes"
	"io"
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

func applyHeaders(h http.Header, m *pb.HeaderMutation) {
	if m == nil {
		return
	}
	for _, k := range m.Remove {
		h.Del(k)
	}
	for _, v := range m.Set {
		h.Del(v.Key)
		for _, value := range v.Values {
			h.Add(v.Key, value)
		}
	}
	for _, v := range m.Append {
		for _, value := range v.Values {
			h.Add(v.Key, value)
		}
	}
}
func clearFraming(h http.Header) {
	for _, k := range []string{"Content-Length", "Transfer-Encoding", "Trailer", "ETag", "Content-MD5", "Digest"} {
		h.Del(k)
	}
}
func replaceRequestBody(req *http.Request, body []byte, mutation bool) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	req.GetBody = nil
	if mutation {
		clearFraming(req.Header)
		req.Trailer = make(http.Header)
	}
}
func replaceResponseBody(response *http.Response, body []byte) {
	if response.Body != nil {
		_ = response.Body.Close()
	}
	response.Body = io.NopCloser(bytes.NewReader(body))
	response.ContentLength = int64(len(body))
	clearFraming(response.Header)
	response.Trailer = make(http.Header)
}
