package interceptors

import (
	"errors"
	"net/http"
	"strings"
	"time"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	rest "github.com/distributed-programming-2026/gateway/api/client/interceptorrest"
	"github.com/distributed-programming-2026/gateway/internal/gateway/app"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func New(definitions map[string]config.Interceptor) (result map[string]app.Interceptor, closeClients func() error, err error) {
	clients := map[string]app.Interceptor{}
	var closers []func() error
	closeAll := func() error {
		var result error
		for _, closeClient := range closers {
			result = errors.Join(result, closeClient())
		}
		return result
	}
	for id, definition := range definitions {
		c := &Client{timeout: config.Value(definition.Timeout, time.Minute)}
		if definition.Transport == "grpc" {
			conn, err := grpc.NewClient(definition.Address, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(64<<20), grpc.MaxCallSendMsgSize(64<<20)))
			if err != nil {
				_ = closeAll()
				return nil, nil, err
			}
			closers = append(closers, conn.Close)
			c.rpc = pb.NewInterceptorClient(conn)
		} else {
			transport := http.DefaultTransport.(*http.Transport).Clone()
			closers = append(closers, func() error { transport.CloseIdleConnections(); return nil })
			client, err := rest.NewClient(strings.TrimRight(definition.Address, "/"), rest.WithHTTPClient(&http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}))
			if err != nil {
				_ = closeAll()
				return nil, nil, err
			}
			spec, err := rest.GetSpec()
			if err != nil {
				_ = closeAll()
				return nil, nil, err
			}
			c.rest = client
			c.schema = spec.Paths.Find("/gateway/api/v1/intercept").Post.Responses.Status(200).Value.Content["application/json"].Schema.Value
		}
		clients[id] = c
	}
	return clients, closeAll, nil
}
