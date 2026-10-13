package integration_tests

import (
	"context"
	"crypto/sha1" // #nosec G505 -- Test fixture for legacy htpasswd verification.
	"encoding/base64"
	"fmt"
	"net/http"
	"sync/atomic"
	"testing"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"
)

func TestLocalBasicAuth(t *testing.T) {
	digest := sha1.Sum([]byte("password")) // #nosec G401 -- Legacy htpasswd fixture.
	s := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: private
routes:
  - pattern: '^/'
    directResponse: {status: 200, inlineString: private}
    basicAuth:
      users: ['user:{SHA}%s']
`, base64.StdEncoding.EncodeToString(digest[:])))
	status, h, _ := request(t, s, "GET", "/", "", nil)
	require.EqualValues(t, 401, status,
		"Basic Auth challenge missing")
	require.NotEqual(t, "", h.Get("WWW-Authenticate"),
		"Basic Auth challenge missing")

	credentials := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:password"))
	status, _, body := request(t, s, "GET", "/", "", http.Header{authorizationHeader: []string{credentials}})
	require.EqualValues(t, 200, status,
		"Basic Auth rejected valid credentials")
	require.Equal(t, "private", body,
		"Basic Auth rejected valid credentials")

	wrong := "Basic " + base64.StdEncoding.EncodeToString([]byte("user:wrong"))
	status, _, _ = request(t, s, "GET", "/", "", http.Header{authorizationHeader: []string{wrong}})
	require.EqualValues(t, 401, status,
		"Basic Auth accepted invalid credentials")
}

func TestBcryptAuthRunsBeforeInterceptors(t *testing.T) {
	hash, err := bcrypt.GenerateFromPassword([]byte("password"), bcrypt.MinCost)
	require.NoError(t, err)
	var calls atomic.Int32
	address := grpcServer(t, func(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		calls.Add(1)
		return ok(&pb.ContinueResponse{}), nil
	})
	s := gateway(t, fmt.Sprintf("kind: gateway/service/v1\nname: processor\ninterceptor: {address: '%s'}\n---\nkind: gateway/service/v1\nname: private\nroutes: [{pattern: '^/', directResponse: {status: 200}, basicAuth: {users: ['user:%s']}, interceptors: [{id: processor}]}]\n", address, hash))
	for _, tc := range []struct {
		user, password string
		want           int
	}{{"wrong", "password", 401}, {"user", "wrong", 401}, {"user", "password", 200}} {
		credential := "Basic " + base64.StdEncoding.EncodeToString([]byte(tc.user+":"+tc.password))
		status, _, _ := request(t, s, "GET", "/", "", http.Header{authorizationHeader: []string{credential}})
		require.Equal(t, tc.want, status)
	}
	require.EqualValues(t, 1, calls.Load())
}
