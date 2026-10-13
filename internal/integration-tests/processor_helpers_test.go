package integration_tests

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/distributed-programming-2026/gateway/internal/gateway/config"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/encoding/protojson"
)

func processorServer(t *testing.T, transport string, f interceptorFunc) string {
	t.Helper()
	if transport == grpcTransport {
		return grpcServer(t, f)
	}
	s := restServer(t, func(input map[string]any) any {
		phase := input["eventType"].(string)
		data, err := json.Marshal(map[string]any{"context": input["context"], phase: input["event"]})
		if !assert.NoError(t, err) {
			return nil
		}
		event := &pb.InterceptRequest{}
		if !assert.NoError(t, protojson.Unmarshal(data, event)) {
			return nil
		}
		decision, err := f(context.Background(), event)
		if !assert.NoError(t, err) {
			return nil
		}
		data, err = (protojson.MarshalOptions{EmitDefaultValues: true}).Marshal(decision)
		if !assert.NoError(t, err) {
			return nil
		}
		var wrapper map[string]map[string]any
		if !assert.NoError(t, json.Unmarshal(data, &wrapper)) {
			return nil
		}
		for kind, payload := range wrapper {
			payload["responseType"] = kind
			return payload
		}
		return nil
	})
	return s.URL
}
func interceptedGateway(t *testing.T, upstream, mode, routeFields string, transports []string, processors ...interceptorFunc) *httptest.Server {
	t.Helper()
	var yaml strings.Builder
	for index, f := range processors {
		address := processorServer(t, transports[index], f)
		fmt.Fprintf(&yaml, "kind: gateway/service/v1\nname: processor-%d\ninterceptor: {address: '%s', transport: %s}\n---\n", index, address, transports[index])
	}
	fmt.Fprintf(&yaml, "kind: gateway/service/v1\nname: backend\nroutes:\n  - pattern: '^/'\n    target: '%s'\n%s\n    interceptors:\n", upstream, routeFields)
	for index := range processors {
		fmt.Fprintf(&yaml, "      - id: processor-%d\n        parameters: {configured: value}\n        processingMode: %s\n", index, mode)
	}
	return gateway(t, yaml.String())
}
func eventPhase(r *pb.InterceptRequest) string {
	switch {
	case r.GetRequestHeaders() != nil:
		return config.RequestHeaders
	case r.GetRequestBody() != nil:
		return config.RequestBody
	case r.GetRequestTrailers() != nil:
		return config.RequestTrailers
	case r.GetResponseHeaders() != nil:
		return config.ResponseHeaders
	case r.GetResponseBody() != nil:
		return config.ResponseBody
	case r.GetResponseTrailers() != nil:
		return config.ResponseTrailers
	default:
		return "unknown"
	}
}

const allProcessing = "{requestBody: buffered, responseHeaders: true, responseBody: buffered, requestTrailers: true, responseTrailers: true}"
