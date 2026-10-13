package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDefaultsAndInterceptorMerge(t *testing.T) {
	dir := t.TempDir()
	file(t, dir, "low.yml", `kind: gateway/v1
weight: -1
---
kind: gateway/service/v1
name: processor
weight: -1
interceptor: {address: 'localhost:1234', transport: grpc, timeout: 1s}
`)
	file(t, dir, "high.yaml", `kind: gateway/service/v1
name: processor
weight: 1
interceptor: {timeout: 0s}
---
kind: gateway/service/v1
name: backend
routes: [{patterns: ['^/a$', '^/b$'], directResponse: {status: 200}, interceptors: [{id: processor}]}]
`)
	c, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, 8000, c.Gateway.Port)
	require.Equal(t, 15000, c.Gateway.HealthzPort)
	require.Equal(t, 8082, c.Gateway.GatewayHealthHost)
	require.Equal(t, "localhost:1234", c.Interceptors["processor"].Address)
	require.Equal(t, "grpc", c.Interceptors["processor"].Transport)
	require.Zero(t, Value(c.Interceptors["processor"].Timeout, time.Minute))
	mode := c.Routes[0].Interceptors[0].ProcessingMode
	require.True(t, mode.Enabled(RequestHeaders))
	require.False(t, mode.Enabled(ResponseHeaders))
	require.EqualValues(t, 1<<20, *mode.MaxRequestBodyBytes)
	require.EqualValues(t, 1<<20, *mode.MaxResponseBodyBytes)
	require.False(t, mode.Enabled("unknown"))
	file(t, dir, "explicit.yaml", "kind: gateway/v1\nweight: 100\nport: 0\nhealthzPort: 0\ngatewayHealthHost: 0\n")
	c, err = Load(dir)
	require.NoError(t, err)
	require.Zero(t, c.Gateway.Port)
	require.Zero(t, c.Gateway.HealthzPort)
	require.Zero(t, c.Gateway.GatewayHealthHost)
}
func TestSecretsMergeRetainsKeysAndExpandsLists(t *testing.T) {
	dir := t.TempDir()
	file(t, dir, "a.yaml", `kind: gateway/secret/v1
name: auth
weight: min
values: {one: old, two: preserved}
---
kind: gateway/secret/v1
name: auth
weight: max
values: {one: new}
---
kind: gateway/service/v1
name: backend
routes: [{patterns: ['$(GATEWAY_UNSET_PATTERN_12345:-^/a$)', '^/b$'], directResponse: {status: 200}, addHeaders: {X-One: {value: 'secret:auth:one'}, X-Two: {value: 'secret:auth:two'}}}]
`)
	c, err := Load(dir)
	require.NoError(t, err)
	require.Equal(t, "new", c.Routes[0].AddHeaders["X-One"].Value)
	require.Equal(t, "preserved", c.Routes[0].AddHeaders["X-Two"].Value)
	require.Equal(t, []string{"^/a$", "^/b$"}, c.Routes[0].Patterns)
}
