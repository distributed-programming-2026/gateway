package interceptors

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	rest "github.com/distributed-programming-2026/gateway/api/client/interceptorrest"
	"github.com/getkin/kin-openapi/openapi3"
)

type Client struct {
	rpc     pb.InterceptorClient
	rest    *rest.Client
	schema  *openapi3.Schema
	timeout time.Duration
}

func (c *Client) Intercept(ctx context.Context, request *pb.InterceptRequest) (*pb.InterceptResponse, error) {
	if c.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.timeout)
		defer cancel()
	}
	if c.rpc != nil {
		return c.rpc.Intercept(ctx, request)
	}
	b, err := encodeEvent(request)
	if err != nil {
		return nil, err
	}
	response, err := c.rest.InterceptWithBody(ctx, "application/json", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("interceptor transport status %d", response.StatusCode)
	}
	b, err = io.ReadAll(io.LimitReader(response.Body, (64<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 64<<20 {
		return nil, errors.New("interceptor response too large")
	}
	return decodeDecision(b, c.schema)
}
