package integration_tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMethodsHostMatchingAndRedirectLoop(t *testing.T) {
	s := gateway(t, `kind: gateway/service/v1
name: routes
routes:
  - pattern: '^/restricted$'
    allowedMethods: [GET]
    directResponse: {status: 200, inlineString: allowed}
  - pattern: '^/loop$'
    internalRedirect: {old: '^/loop$', new: /loop, maxRedirectCount: 2}
  - pattern: '^/host$'
    headerMatch: [{header: X-Region, valueRegex: '^ru$'}]
    directResponse: {status: 200, inlineString: region}
  - pattern: '^/.*$'
    directResponse: {status: 201, inlineString: fallback}
`)
	status, _, _ := request(t, s, "POST", "/restricted", "", nil)
	require.EqualValues(t, 403, status,
		"forbidden method fell through")

	status, _, body := request(t, s, "HEAD", "/restricted", "", nil)
	require.EqualValues(t, 200, status,
		"HEAD failed")
	require.Equal(t, "", body,
		"HEAD failed")

	status, _, _ = request(t, s, "GET", "/loop", "", nil)
	require.EqualValues(t, 500, status,
		"redirect loop not stopped")

	status, _, body = request(t, s, "GET", "/host", "", http.Header{"X-Region": []string{"en", "ru"}})
	require.EqualValues(t, 200, status,
		"header match failed")
	require.Equal(t, "region", body,
		"header match failed")
}
