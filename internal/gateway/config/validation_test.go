package config

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRejectedDefinitions(t *testing.T) {
	gatewayFields := map[string]string{
		"negative-port": "port: -1", "overflow-port": "healthzPort: 65536", "management": "gatewayPort: 1",
		"bad-internal-header": "internalHeaders: {headers: ['bad header']}", "internal-host": "internalHeaders: {headers: [Host]}",
		"numeric-duration": "proxyTimeouts: {timeout: 0}", "invalid-duration": "proxyTimeouts: {idleTimeout: tomorrow}",
		"removed-csp": "cspHeaders: {}", "removed-maintenance": "maintenance: {}", "unknown": "typo: true",
		"invalid-weight": "weight: max", "float-weight": "weight: 0.5", "null-list": "internalHeaders: {headers: [null]}",
	}
	for name, field := range gatewayFields {
		t.Run(name, func(t *testing.T) { rejected(t, "kind: gateway/v1\n"+field) })
	}
	routes := map[string]string{
		"no-pattern": "target: localhost:80", "two-pattern-fields": "pattern: '^/', patterns: ['^/'], target: localhost:80",
		"bad-pattern": "pattern: '[', target: localhost:80", "bad-rewrite": "pattern: '^/', target: localhost:80, rewrite: {old: '[', new: /}",
		"empty-rewrite":  "pattern: '^/', target: localhost:80, rewrite: {old: '', new: /}",
		"rewrite-limit":  "pattern: '^/', target: localhost:80, rewrite: {old: '^/', new: /, maxRedirectCount: 2}",
		"redirect-limit": "pattern: '^/', internalRedirect: {old: '^/', new: /, maxRedirectCount: 0}",
		"no-action":      "pattern: '^/'", "two-actions": "pattern: '^/', target: localhost:80, directResponse: {status: 200}",
		"target-scheme": "pattern: '^/', target: 'ftp://localhost'", "target-path": "pattern: '^/', target: 'http://localhost/path'",
		"target-user": "pattern: '^/', target: 'http://user@localhost'", "target-query": "pattern: '^/', target: 'http://localhost?a=1'",
		"status": "pattern: '^/', directResponse: {status: 199}", "method": "pattern: '^/', target: localhost:80, allowedMethods: ['bad method']",
		"match-header":          "pattern: '^/', target: localhost:80, headerMatch: [{header: 'bad header'}]",
		"match-regexp":          "pattern: '^/', target: localhost:80, headerMatch: [{header: X-Test, valueRegex: '['}]",
		"add-framing":           "pattern: '^/', target: localhost:80, addHeaders: {Content-Length: {value: '10'}}",
		"add-name":              "pattern: '^/', target: localhost:80, addHeaders: {'bad header': {value: test}}",
		"add-crlf":              "pattern: '^/', target: localhost:80, addHeaders: {X-Test: {value: \"bad\\nvalue\"}}",
		"add-parameter":         "pattern: '^/', target: localhost:80, addHeaders: {X-Test: {pathParametersPattern: '^/{id}$', value: '{missing}'}}",
		"add-pattern":           "pattern: '^/', target: localhost:80, addHeaders: {X-Test: {pathParametersPattern: '[', value: x}}",
		"missing-interceptor":   "pattern: '^/', target: localhost:80, interceptors: [{id: missing}]",
		"body-mode":             "pattern: '^/', target: localhost:80, interceptors: [{id: processor, processingMode: {requestBody: streamed}}]",
		"trailer-mode":          "pattern: '^/', target: localhost:80, interceptors: [{id: processor, processingMode: {requestTrailers: true}}]",
		"response-trailer-mode": "pattern: '^/', target: localhost:80, interceptors: [{id: processor, processingMode: {responseTrailers: true}}]",
		"zero-limit":            "pattern: '^/', target: localhost:80, interceptors: [{id: processor, processingMode: {maxRequestBodyBytes: 0}}]",
		"large-limit":           "pattern: '^/', target: localhost:80, interceptors: [{id: processor, processingMode: {maxResponseBodyBytes: 33554433}}]",
		"http2-buffer":          "pattern: '^/', target: localhost:80, useHTTP2: true, interceptors: [{id: processor, processingMode: {responseBody: buffered}}]",
		"parameter-pattern":     "pattern: '^/', target: localhost:80, interceptors: [{id: processor, pathParametersPattern: '['}]",
		"removed-type":          "pattern: '^/', target: localhost:80, interceptors: [{id: processor, type: external}]",
		"dynamic-target":        "pattern: '^/', target: localhost:80, dynamicTarget: true",
		"empty-auth":            "pattern: '^/', target: localhost:80, basicAuth: {users: []}",
		"auth-record":           "pattern: '^/', target: localhost:80, basicAuth: {users: [user]}",
		"auth-sha":              "pattern: '^/', target: localhost:80, basicAuth: {users: ['user:{SHA}bad']}",
		"auth-hash":             "pattern: '^/', target: localhost:80, basicAuth: {users: ['user:plain']}",
	}
	for name, route := range routes {
		t.Run(name, func(t *testing.T) {
			rejected(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: localhost:1234}\n---\nkind: gateway/service/v1\nname: backend\nroutes: [{%s}]\n", route))
		})
	}
	definitions := map[string]string{
		"empty-service": "kind: gateway/service/v1\nname: empty", "missing-name": "kind: gateway/service/v1\ninterceptor: {address: localhost:1234}",
		"empty-address": "kind: gateway/service/v1\nname: p\ninterceptor: {}",
		"grpc-url":      "kind: gateway/service/v1\nname: p\ninterceptor: {address: 'http://localhost:1234'}",
		"rest-address":  "kind: gateway/service/v1\nname: p\ninterceptor: {address: localhost:1234, transport: rest}",
		"transport":     "kind: gateway/service/v1\nname: p\ninterceptor: {address: localhost:1234, transport: unknown}",
		"secret-name":   "kind: gateway/secret/v1\nvalues: {key: value}", "secret-values": "kind: gateway/secret/v1\nname: s\nvalues: {}",
		"secret-type": "kind: gateway/secret/v1\nname: s\nvalues: {key: 42}", "secret-field": "kind: gateway/secret/v1\nname: s\nunknown: true\nvalues: {key: x}",
		"recursive-secret": "kind: gateway/secret/v1\nname: s\nvalues: {key: 'secret:s:key'}",
		"missing-secret":   "kind: gateway/service/v1\nname: p\ninterceptor: {address: 'secret:s:key'}",
		"malformed-secret": "kind: gateway/service/v1\nname: p\ninterceptor: {address: 'secret:s'}",
		"missing-env":      "kind: gateway/service/v1\nname: p\ninterceptor: {address: '$(GATEWAY_MISSING_VARIABLE_12345)'}",
		"malformed-yaml":   "kind: [", "unknown-kind": "kind: other/v1",
	}
	for name, body := range definitions {
		t.Run(name, func(t *testing.T) { rejected(t, body) })
	}
}
func rejected(t *testing.T, body string) {
	t.Helper()
	dir := t.TempDir()
	file(t, dir, "invalid.yaml", body)
	_, err := Load(dir)
	require.Error(t, err)
}
func TestMissingAndEmptyConfiguration(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.yaml"))
	require.Error(t, err)
	_, err = Load(t.TempDir())
	require.Error(t, err)
	_, err = Load()
	require.Error(t, err)
}
