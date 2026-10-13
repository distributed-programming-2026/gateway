// Package config loads and compiles an immutable gateway configuration.
package config

import (
	"errors"
	"net/url"
	"regexp"
	"time"

	"go.yaml.in/yaml/v3"
)

type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	if n.Tag != "!!str" {
		return errors.New("duration must be a string")
	}
	v, err := time.ParseDuration(n.Value)
	if err != nil || v < 0 {
		return errors.New("invalid duration")
	}
	*d = Duration(v)
	return nil
}
func Value(d *Duration, fallback time.Duration) time.Duration {
	if d == nil {
		return fallback
	}
	return time.Duration(*d)
}

type Gateway struct {
	Kind              string `yaml:"kind"`
	Weight            int    `yaml:"weight"`
	Port              int    `yaml:"port"`
	HealthzPort       int    `yaml:"healthzPort"`
	GatewayPort       int    `yaml:"gatewayPort"`
	GatewayHealthHost int    `yaml:"gatewayHealthHost"`
	ProxyTimeouts     struct {
		Timeout     *Duration `yaml:"timeout"`
		IdleTimeout *Duration `yaml:"idleTimeout"`
	} `yaml:"proxyTimeouts"`
	InternalHeaders struct {
		Headers []string `yaml:"headers"`
	} `yaml:"internalHeaders"`
}
type Interceptor struct {
	Address   string    `yaml:"address"`
	Transport string    `yaml:"transport"`
	Timeout   *Duration `yaml:"timeout"`
}
type Service struct {
	Kind        string       `yaml:"kind"`
	Weight      int          `yaml:"weight"`
	Name        string       `yaml:"name"`
	Interceptor *Interceptor `yaml:"interceptor"`
	Routes      []Route      `yaml:"routes"`
}
type Rule struct {
	Old              string         `yaml:"old"`
	New              string         `yaml:"new"`
	MaxRedirectCount *int           `yaml:"maxRedirectCount"`
	RE               *regexp.Regexp `yaml:"-"`
}
type HeaderMatch struct {
	Header     string         `yaml:"header"`
	ValueRegex string         `yaml:"valueRegex"`
	RE         *regexp.Regexp `yaml:"-"`
}
type Header struct {
	Value                 string         `yaml:"value"`
	Always                bool           `yaml:"always"`
	PathParametersPattern string         `yaml:"pathParametersPattern"`
	RE                    *regexp.Regexp `yaml:"-"`
}
type DirectResponse struct {
	Status       int    `yaml:"status"`
	InlineString string `yaml:"inlineString"`
}
type Invocation struct {
	ID                    string            `yaml:"id"`
	PathParametersPattern string            `yaml:"pathParametersPattern"`
	Parameters            map[string]string `yaml:"parameters"`
	ProcessingMode        ProcessingMode    `yaml:"processingMode"`
	RE                    *regexp.Regexp    `yaml:"-"`
}
type BasicAuth struct {
	Users []string `yaml:"users"`
}
type Route struct {
	Pattern            string            `yaml:"pattern"`
	Patterns           []string          `yaml:"patterns"`
	Target             string            `yaml:"target"`
	Rewrite            *Rule             `yaml:"rewrite"`
	InternalRedirect   *Rule             `yaml:"internalRedirect"`
	Timeout            *Duration         `yaml:"timeout"`
	IdleTimeout        *Duration         `yaml:"idleTimeout"`
	HeaderMatch        []HeaderMatch     `yaml:"headerMatch"`
	AllowedMethods     []string          `yaml:"allowedMethods"`
	Interceptors       []Invocation      `yaml:"interceptors"`
	DirectResponse     *DirectResponse   `yaml:"directResponse"`
	DisableHostRewrite bool              `yaml:"disableHostRewrite"`
	UseHTTP2           bool              `yaml:"useHTTP2"`
	AddHeaders         map[string]Header `yaml:"addHeaders"`
	BasicAuth          *BasicAuth        `yaml:"basicAuth"`
	REs                []*regexp.Regexp  `yaml:"-"`
	URL                *url.URL          `yaml:"-"`
	Service            string            `yaml:"-"`
}
type Config struct {
	Gateway      Gateway
	Routes       []Route
	Interceptors map[string]Interceptor
}
type document struct {
	data           map[string]any
	file           string
	weight         int
	secretPriority int
}
