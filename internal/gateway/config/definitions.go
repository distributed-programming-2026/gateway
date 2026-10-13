package config

import (
	"errors"
	"net/url"
	"strings"

	"golang.org/x/net/http/httpguts"
)

func compileGateway(g *Gateway, global map[string]any) error {
	// Apply defaults only after merging. Presence, including explicit zero/false, matters.
	if _, ok := global["port"]; !ok {
		g.Port = 8000
	}
	if _, ok := global["healthzPort"]; !ok {
		g.HealthzPort = 15000
	}
	if _, ok := global["gatewayHealthHost"]; !ok {
		g.GatewayHealthHost = 8082
	}
	for _, port := range []int{g.Port, g.HealthzPort, g.GatewayHealthHost, g.GatewayPort} {
		if port < 0 || port > 65535 {
			return errors.New("invalid port")
		}
	}
	if g.GatewayPort != 0 {
		return errors.New("gatewayPort management API has no defined contract; leave it unset")
	}
	for _, h := range g.InternalHeaders.Headers {
		if !httpguts.ValidHeaderFieldName(h) || strings.EqualFold(h, "Host") {
			return errors.New("invalid internal header")
		}
	}
	return nil
}

func compileInterceptor(i *Interceptor) error {
	if i.Transport == "" {
		i.Transport = "grpc"
	}
	if i.Address == "" {
		return errors.New("interceptor address required")
	}
	switch i.Transport {
	case "grpc":
		if strings.Contains(i.Address, "://") {
			return errors.New("grpc interceptor address must be host:port")
		}
	case "rest":
		if _, err := targetURL(i.Address, false); err != nil {
			return errors.New("invalid REST address")
		}
	default:
		return errors.New("invalid interceptor transport")
	}
	return nil
}
func targetURL(s string, implicit bool) (*url.URL, error) {
	if implicit && !strings.Contains(s, "://") {
		s = "http://" + s
	}
	u, err := url.Parse(s)
	if err != nil || u == nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, errors.New("target must be HTTP(S) authority")
	}
	return u, nil
}
