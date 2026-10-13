package config

import (
	"errors"
)

func compileMode(m *ProcessingMode, http2 bool) error {
	if m.RequestBody == "" {
		m.RequestBody = BodyNone
	}
	if m.ResponseBody == "" {
		m.ResponseBody = BodyNone
	}
	if (m.RequestBody != BodyNone && m.RequestBody != BodyBuffered) || (m.ResponseBody != BodyNone && m.ResponseBody != BodyBuffered) {
		return errors.New("invalid body mode")
	}
	if m.MaxRequestBodyBytes == nil {
		value := int64(1 << 20)
		m.MaxRequestBodyBytes = &value
	}
	if m.MaxResponseBodyBytes == nil {
		value := int64(1 << 20)
		m.MaxResponseBodyBytes = &value
	}
	if *m.MaxRequestBodyBytes < 1 || *m.MaxResponseBodyBytes < 1 || *m.MaxRequestBodyBytes > 32<<20 || *m.MaxResponseBodyBytes > 32<<20 {
		return errors.New("body limit must be between 1 and 33554432 bytes")
	}
	if (m.RequestTrailers && m.RequestBody != BodyBuffered) || (m.ResponseTrailers && m.ResponseBody != BodyBuffered) {
		return errors.New("trailers require buffered body")
	}
	if http2 && (m.RequestBody == BodyBuffered || m.ResponseBody == BodyBuffered) {
		return errors.New("HTTP2 body processing unsupported")
	}
	return nil
}
