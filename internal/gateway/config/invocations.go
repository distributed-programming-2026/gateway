package config

import (
	"fmt"
)

func compileInvocations(r *Route, c *Config) error {
	for i := range r.Interceptors {
		v := &r.Interceptors[i]
		if _, ok := c.Interceptors[v.ID]; !ok {
			return fmt.Errorf("unknown interceptor %s", v.ID)
		}
		if v.PathParametersPattern != "" {
			re, err := ParametersRegexp(v.PathParametersPattern)
			if err != nil {
				return err
			}
			v.RE = re
		}
		if err := compileMode(&v.ProcessingMode, r.UseHTTP2); err != nil {
			return err
		}
	}

	return nil
}
