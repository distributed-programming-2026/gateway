package config

import (
	"errors"
	"fmt"
	"strings"
)

func mergeSecret(d document, secrets map[string]map[string]string) error {
	for k := range d.data {
		if k != "kind" && k != "weight" && k != "name" && k != "values" {
			return fmt.Errorf("%s: unknown secret field %s", d.file, k)
		}
	}
	name, ok := d.data["name"].(string)
	if !ok || name == "" {
		return errors.New("secret name required")
	}
	values, ok := d.data["values"].(map[string]any)
	if !ok || len(values) == 0 {
		return errors.New("secret values required")
	}
	if secrets[name] == nil {
		secrets[name] = map[string]string{}
	}
	for k, v := range values {
		str, ok := v.(string)
		if !ok {
			return errors.New("secret must be string")
		}
		expanded, err := env(str)
		if err != nil {
			return err
		}
		if strings.HasPrefix(expanded, "secret:") {
			return errors.New("recursive secret unsupported")
		}
		secrets[name][k] = expanded
	}
	return nil
}
