package config

import (
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func compileMatches(r *Route) error {
	for i := range r.HeaderMatch {
		h := &r.HeaderMatch[i]
		if !httpguts.ValidHeaderFieldName(h.Header) {
			return errors.New("invalid header match")
		}
		re, err := regexp.Compile(h.ValueRegex)
		if err != nil {
			return errors.New("invalid header regexp")
		}
		h.RE = re
	}
	for _, m := range r.AllowedMethods {
		if !httpguts.ValidHeaderFieldName(m) {
			return errors.New("invalid HTTP method")
		}
	}

	return nil
}

func (r *Route) Matches(req *http.Request) bool {
	match := false
	for _, re := range r.REs {
		if re.MatchString(req.URL.Path) {
			match = true
			break
		}
	}
	if !match {
		return false
	}
	for _, h := range r.HeaderMatch {
		values := req.Header.Values(h.Header)
		if strings.EqualFold(h.Header, "Host") {
			values = []string{req.Host}
		}
		ok := false
		for _, v := range values {
			if h.RE.MatchString(v) {
				ok = true
			}
		}
		if !ok {
			return false
		}
	}
	return true
}
func (r *Route) MethodAllowed(method string) bool {
	if len(r.AllowedMethods) == 0 {
		return true
	}
	for _, m := range r.AllowedMethods {
		if m == method || (m == http.MethodGet && method == http.MethodHead) {
			return true
		}
	}
	return false
}
func Address(port int) string { return ":" + strconv.Itoa(port) }
