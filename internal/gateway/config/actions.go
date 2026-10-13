package config

import (
	"errors"
)

func compileAction(r *Route) error {
	if r.Target != "" && r.DirectResponse != nil {
		return errors.New("target and directResponse conflict")
	}
	if r.Target == "" && r.DirectResponse == nil && r.InternalRedirect == nil {
		return errors.New("route action required")
	}
	if r.Target != "" {
		var err error
		r.URL, err = targetURL(r.Target, true)
		if err != nil {
			return err
		}
	}
	if r.DirectResponse != nil && (r.DirectResponse.Status < 200 || r.DirectResponse.Status > 599) {
		return errors.New("invalid directResponse status")
	}

	return nil
}
