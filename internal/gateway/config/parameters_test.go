package config

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParameterRegexpSyntaxAndHeaderTemplates(t *testing.T) {
	for _, tc := range []struct {
		pattern, path string
		want          map[string]string
	}{
		{`^/{id}/(?<version>[0-9]{2})$`, "/42/01", map[string]string{"id": "42", "version": "01"}},
		{`^/\Q{literal}\E/{id}$`, "/{literal}/42", map[string]string{"id": "42"}},
		{`^/[{}]+/{id}$`, "/{}/42", map[string]string{"id": "42"}},
		{`^/\{literal\}/{id}$`, "/{literal}/42", map[string]string{"id": "42"}},
		{`^/{id}$`, "/a/b", map[string]string{}},
		{`^/\Q{literal}`, "/{literal}", map[string]string{}},
	} {
		t.Run(tc.pattern, func(t *testing.T) {
			re, err := ParametersRegexp(tc.pattern)
			require.NoError(t, err)
			require.Equal(t, tc.want, Parameters(re, tc.path))
		})
	}
	for _, pattern := range []string{`/{id}/{id}`, `/{id}/(?P<id>.*)`, `[`, `[\`} {
		_, err := ParametersRegexp(pattern)
		require.Error(t, err)
	}
	require.Empty(t, Parameters(nil, "/"))
	re, err := ParametersRegexp(`^/{id}$`)
	require.NoError(t, err)
	value, ok := HeaderValue(Header{Value: "{{id}}={id}", RE: re}, "/42")
	require.True(t, ok)
	require.Equal(t, "{id}=42", value)
	_, ok = HeaderValue(Header{Value: "{id}", RE: re}, "/42/more")
	require.False(t, ok)
	value, ok = HeaderValue(Header{Value: "{literal}"}, "/")
	require.True(t, ok)
	require.Equal(t, "{literal}", value)
}
