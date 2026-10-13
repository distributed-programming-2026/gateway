package config

import (
	"time"

	"github.com/stretchr/testify/require"
)

func (s *ConfigSuite) TestMergePreservesZeroEmptyListAndRouteWeights() {
	t := s.T()
	dir := s.dir
	file(t, dir, "a.yaml", `kind: gateway/v1
weight: 100
proxyTimeouts: {timeout: 0s}
internalHeaders: {headers: []}
---
kind: gateway/service/v1
name: same
weight: 100
routes:
  - pattern: '^/first$'
    directResponse: {status: 200, inlineString: first}
  - pattern: '^/second$'
    directResponse: {status: 200, inlineString: second}
`)
	file(t, dir, "b.yaml", `kind: gateway/v1
weight: -100
proxyTimeouts: {timeout: 1s, idleTimeout: 2s}
internalHeaders: {headers: [X-Old]}
---
kind: gateway/service/v1
name: same
weight: -100
routes:
  - pattern: '^/.*$'
    directResponse: {status: 404, inlineString: fallback}
`)
	c, err := Load(dir)
	require.NoError(t, err)
	require.EqualValues(t, 0, Value(c.Gateway.ProxyTimeouts.Timeout, time.Minute),
		"incorrect merged configuration %+v", c)
	require.Equal(t, 2*time.Second, Value(c.Gateway.ProxyTimeouts.IdleTimeout, 0),
		"incorrect merged configuration %+v", c)
	require.EqualValues(t, 0, len(c.Gateway.InternalHeaders.Headers),
		"incorrect merged configuration %+v", c)
	require.EqualValues(t, 3, len(c.Routes),
		"incorrect merged configuration %+v", c)
	require.Equal(t, "^/first$", c.Routes[0].Pattern,
		"incorrect merged configuration %+v", c)
	require.Equal(t, "^/second$", c.Routes[1].Pattern,
		"incorrect merged configuration %+v", c)
	require.Equal(t, "^/.*$", c.Routes[2].Pattern,
		"incorrect merged configuration %+v", c)
}
