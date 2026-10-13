package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

var envRE = regexp.MustCompile(`\$\(([A-Za-z_][A-Za-z_0-9]*)(?::-([^)]*))?\)`)

func env(s string) (string, error) {
	var failure error
	s = envRE.ReplaceAllStringFunc(s, func(token string) string {
		m := envRE.FindStringSubmatch(token)
		v, ok := os.LookupEnv(m[1])
		if ok && v != "" {
			return v
		}
		if strings.Contains(token, ":-") {
			return m[2]
		}
		failure = fmt.Errorf("environment variable %s required", m[1])
		return ""
	})
	return s, failure
}
func expand(v any, secrets map[string]map[string]string) (any, error) {
	switch x := v.(type) {
	case string:
		s, err := env(x)
		if err != nil {
			return nil, err
		}
		if strings.HasPrefix(s, "secret:") {
			parts := strings.Split(s, ":")
			if len(parts) != 3 {
				return nil, errors.New("invalid secret reference")
			}
			v, ok := secrets[parts[1]][parts[2]]
			if !ok {
				return nil, fmt.Errorf("missing secret %s:%s", parts[1], parts[2])
			}
			return v, nil
		}
		return s, nil
	case map[string]any:
		m := map[string]any{}
		for k, v := range x {
			value, err := expand(v, secrets)
			if err != nil {
				return nil, err
			}
			m[k] = value
		}
		return m, nil
	case []any:
		a := make([]any, len(x))
		for i, v := range x {
			value, err := expand(v, secrets)
			if err != nil {
				return nil, err
			}
			a[i] = value
		}
		return a, nil
	}
	return v, nil
}
func decode(v, dst any, secrets map[string]map[string]string) error {
	value, err := expand(v, secrets)
	if err != nil {
		return err
	}
	b, err := yaml.Marshal(value)
	if err != nil {
		return err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	if err = dec.Decode(dst); err != nil {
		return errors.New("invalid fields or values in definition")
	}
	return nil
}
