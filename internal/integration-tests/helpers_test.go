package integration_tests

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"github.com/distributed-programming-2026/gateway/internal/gateway/infra/server"
	"google.golang.org/grpc"
)

func load(t *testing.T, yaml string) *config.Config {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte(yaml), 0o600))

	c, err := config.Load(file)
	require.NoError(t, err)

	return c
}
func gateway(t *testing.T, yaml string) *httptest.Server {
	t.Helper()
	r, err := server.New(load(t, yaml))
	require.NoError(t, err)

	t.Cleanup(func() {
		assert.NoError(t, r.Close())
	})
	s := httptest.NewUnstartedServer(r.Handler)
	s.Config.Protocols = server.Protocols()
	s.Start()
	t.Cleanup(s.Close)
	return s
}
func request(t *testing.T, s *httptest.Server, method, path, body string, headers http.Header) (statusCode int, responseHeaders http.Header, responseBody string) {
	t.Helper()
	req, err := http.NewRequest(method, s.URL+path, strings.NewReader(body))
	require.NoError(t, err)

	req.Header = headers.Clone()
	if req.Header == nil {
		req.Header = make(http.Header)
	}
	client := s.Client()
	client.Timeout = 3 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := client.Do(req)
	require.NoError(t, err)

	defer func() { _ = response.Body.Close() }()
	data, err := io.ReadAll(response.Body)
	require.NoError(t, err)

	return response.StatusCode, response.Header, string(data)
}

type interceptorFunc func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error)
type grpcInterceptor struct {
	pb.UnimplementedInterceptorServer
	f interceptorFunc
}

func (s *grpcInterceptor) Intercept(ctx context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
	return s.f(ctx, r)
}
func grpcServer(t *testing.T, f interceptorFunc) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	s := grpc.NewServer()
	pb.RegisterInterceptorServer(s, &grpcInterceptor{f: f})
	go func() { _ = s.Serve(l) }()
	t.Cleanup(s.Stop)
	return l.Addr().String()
}
func ok(m *pb.ContinueResponse) *pb.InterceptResponse {
	return &pb.InterceptResponse{Result: &pb.InterceptResponse_ContinueResponse{ContinueResponse: m}}
}
func set(key string, values ...string) *pb.HeaderMutation {
	return &pb.HeaderMutation{Set: []*pb.HeaderValues{{Key: key, Values: values}}}
}
func getHeader(headers []*pb.Header, key string) string {
	for _, h := range headers {
		if strings.EqualFold(h.Key, key) {
			return h.Value
		}
	}
	return ""
}
func restServer(t *testing.T, f func(map[string]any) any) *httptest.Server {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/gateway/api/v1/intercept", r.URL.Path,
			"invalid REST endpoint")
		assert.Equal(t, "POST", r.Method,
			"invalid REST endpoint")

		var input map[string]any
		assert.NoError(t, json.NewDecoder(r.Body).Decode(&input))

		w.Header().Set("Content-Type", "application/json")
		assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"payload": f(input)}))
	}))
	t.Cleanup(s.Close)
	return s
}

const multiHeader = "X-Multi"
const responseLimitScenario = "response-limit"
const jsonHeaderKey = "key"

const jsonSet = "set"
const timeoutScenario = "timeout"

const authorizationHeader = "Authorization"

const jsonHeaderValues = "values"

const grpcTransport = "grpc"

const restTransport = "rest"

const firstPatternPath = "/one"

const tenantHost = "tenant.example"
