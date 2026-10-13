package config

import (
	"errors"
	"fmt"
)

func Load(paths ...string) (*Config, error) {
	docs, err := readDocuments(paths)
	if err != nil {
		return nil, err
	}
	secrets := map[string]map[string]string{}
	global := map[string]any{"kind": "gateway/v1"}
	declarations := map[string]map[string]any{}
	for _, d := range docs {
		switch d.data["kind"] {
		case "gateway/secret/v1":
			if err = mergeSecret(d, secrets); err != nil {
				return nil, err
			}
		case "gateway/v1":
			merge(global, d.data)
		case "gateway/service/v1":
			if err = mergeDeclaration(d, declarations); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%s: unknown kind", d.file)
		}
	}
	c := &Config{Interceptors: map[string]Interceptor{}}
	if err = decode(global, &c.Gateway, secrets); err != nil {
		return nil, fmt.Errorf("gateway definition: %w", err)
	}
	if err = compileGateway(&c.Gateway, global); err != nil {
		return nil, err
	}
	for name, data := range declarations {
		var i Interceptor
		if err = decode(data, &i, secrets); err != nil {
			return nil, fmt.Errorf("interceptor %s: %w", name, err)
		}
		if err = compileInterceptor(&i); err != nil {
			return nil, err
		}
		c.Interceptors[name] = i
	}
	if err = c.loadRoutes(docs, secrets); err != nil {
		return nil, err
	}
	return c, nil
}
func mergeDeclaration(d document, declarations map[string]map[string]any) error {
	name, ok := d.data["name"].(string)
	if !ok || name == "" {
		return errors.New("service name required")
	}
	if declaration, ok := d.data["interceptor"].(map[string]any); ok {
		if declarations[name] == nil {
			declarations[name] = map[string]any{}
		}
		merge(declarations[name], declaration)
	}
	return nil
}
func (c *Config) loadRoutes(docs []document, secrets map[string]map[string]string) error {
	for n := len(docs) - 1; n >= 0; n-- {
		d := docs[n]
		if d.data["kind"] != "gateway/service/v1" {
			continue
		}
		var service Service
		if err := decode(d.data, &service, secrets); err != nil {
			return fmt.Errorf("%s: %w", d.file, err)
		}
		if len(service.Routes) == 0 && service.Interceptor == nil {
			return errors.New("service needs routes or interceptor")
		}
		for index, r := range service.Routes {
			r.Service = service.Name
			if err := compile(&r, c); err != nil {
				return fmt.Errorf("service %s route %d: %w", service.Name, index, err)
			}
			c.Routes = append(c.Routes, r)
		}
	}
	return nil
}
