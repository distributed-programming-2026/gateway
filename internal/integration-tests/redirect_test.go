package integration_tests

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExplicitRedirectLimitCanExceedDefault(t *testing.T) {
	var yaml strings.Builder
	yaml.WriteString("kind: gateway/service/v1\nname: chain\nroutes:\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&yaml, "  - pattern: '^/step-%d$'\n    internalRedirect: {old: '^/step-%d$', new: '/step-%d', maxRedirectCount: 20}\n", i, i, i+1)
	}
	yaml.WriteString("  - pattern: '^/step-12$'\n    directResponse: {status: 200, inlineString: completed}\n")
	s := gateway(t, yaml.String())
	status, _, body := request(t, s, "GET", "/step-0", "", nil)
	require.EqualValues(t, 200, status,
		"explicit limit ignored: %d %s", status, body)
	require.Equal(t, "completed", body,
		"explicit limit ignored: %d %s", status, body)
}
