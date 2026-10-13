package config

import (
	"errors"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func compileHeaders(r *Route) error {
	for k, h := range r.AddHeaders {
		if !httpguts.ValidHeaderFieldName(k) || !httpguts.ValidHeaderFieldValue(h.Value) || strings.EqualFold(k, "Content-Length") || strings.EqualFold(k, "Transfer-Encoding") || strings.EqualFold(k, "Trailer") {
			return errors.New("invalid response header")
		}
		if h.PathParametersPattern != "" {
			re, err := ParametersRegexp(h.PathParametersPattern)
			if err != nil {
				return err
			}
			h.RE = re
			template := strings.ReplaceAll(strings.ReplaceAll(h.Value, "{{", ""), "}}", "")
			for _, match := range placeholderRE.FindAllStringSubmatch(template, -1) {
				if re.SubexpIndex(match[1]) < 0 {
					return errors.New("unknown header path parameter")
				}
			}
		}
		r.AddHeaders[k] = h
	}

	return nil
}
