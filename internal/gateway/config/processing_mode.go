// Package config loads and compiles an immutable gateway configuration.
package config

type ProcessingMode struct {
	RequestHeaders       *bool  `yaml:"requestHeaders"`
	ResponseHeaders      bool   `yaml:"responseHeaders"`
	RequestBody          string `yaml:"requestBody"`
	ResponseBody         string `yaml:"responseBody"`
	RequestTrailers      bool   `yaml:"requestTrailers"`
	ResponseTrailers     bool   `yaml:"responseTrailers"`
	MaxRequestBodyBytes  *int64 `yaml:"maxRequestBodyBytes"`
	MaxResponseBodyBytes *int64 `yaml:"maxResponseBodyBytes"`
}

func (m ProcessingMode) Enabled(phase string) bool {
	switch phase {
	case RequestHeaders:
		return m.RequestHeaders == nil || *m.RequestHeaders
	case ResponseHeaders:
		return m.ResponseHeaders
	case RequestBody:
		return m.RequestBody == BodyBuffered
	case ResponseBody:
		return m.ResponseBody == BodyBuffered
	case RequestTrailers:
		return m.RequestTrailers
	case ResponseTrailers:
		return m.ResponseTrailers
	}
	return false
}

const (
	RequestHeaders   = "requestHeaders"
	RequestBody      = "requestBody"
	RequestTrailers  = "requestTrailers"
	ResponseHeaders  = "responseHeaders"
	ResponseBody     = "responseBody"
	ResponseTrailers = "responseTrailers"
	BodyBuffered     = "buffered"
	BodyNone         = "none"
)
