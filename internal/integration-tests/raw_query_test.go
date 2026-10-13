package integration_tests

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawQueryIsPreservedExactly(t *testing.T) {
	query := "b=two%20words&a=1&a=2&invalid=%zz"
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, query, r.URL.RawQuery,
			"raw query changed: %q", r.URL.RawQuery)

		w.WriteHeader(204)
	}))
	defer backend.Close()
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: backend\nroutes: [{pattern: '^/', target: '%s'}]\n", backend.URL))
	status, _, _ := request(t, s, "GET", "/?"+query, "", nil)
	require.EqualValues(t, 204, status,
		status)
}
