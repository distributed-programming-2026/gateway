package config

import (
	"errors"
	"regexp"
)

func compilePatterns(r *Route) error {
	if (r.Pattern == "") == (len(r.Patterns) == 0) {
		return errors.New("exactly one of pattern/patterns required")
	}
	patterns := r.Patterns
	if r.Pattern != "" {
		patterns = []string{r.Pattern}
	}
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return errors.New("invalid route regexp")
		}
		r.REs = append(r.REs, re)
	}
	for _, rule := range []*Rule{r.Rewrite, r.InternalRedirect} {
		if rule == nil {
			continue
		}
		re, err := regexp.Compile(rule.Old)
		if err != nil || rule.Old == "" {
			return errors.New("invalid rewrite regexp")
		}
		rule.RE = re
	}
	if r.Rewrite != nil && r.Rewrite.MaxRedirectCount != nil {
		return errors.New("maxRedirectCount only for internalRedirect")
	}
	if r.InternalRedirect != nil && r.InternalRedirect.MaxRedirectCount != nil && *r.InternalRedirect.MaxRedirectCount <= 0 {
		return errors.New("redirect limit must be positive")
	}

	return nil
}
