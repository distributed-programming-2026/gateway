package config

import (
	"github.com/stretchr/testify/require"
)

func (s *ConfigSuite) TestSecretsAndEnvironment() {
	t := s.T()
	t.Setenv("GATEWAY_TEST_VALUE", "token")
	dir := t.TempDir()
	file(t, dir, "a.yaml", `kind: gateway/secret/v1
name: credentials
weight: min
values: {key: old, preserved: literal}
---
kind: gateway/secret/v1
name: credentials
weight: max
values: {key: '$(GATEWAY_TEST_VALUE)'}
---
kind: gateway/service/v1
name: interceptor
interceptor: {address: localhost:1234}
---
kind: gateway/service/v1
name: route
routes:
  - pattern: '^/'
    directResponse: {status: 200, inlineString: ok}
    interceptors:
      - id: interceptor
        parameters: {key: 'secret:credentials:key', fallback: '$(UNSET:-default)'}
`)
	c, err := Load(dir)
	require.NoError(t, err)

	p := c.Routes[0].Interceptors[0].Parameters
	require.Equal(t, "token", p["key"],
		p)
	require.Equal(t, "default", p["fallback"],
		p)
}
