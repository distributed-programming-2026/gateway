package app

import (
	"context"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
)

// Interceptor executes one phase. Its decision is applied before the next call.
type Interceptor interface {
	Intercept(context.Context, *pb.InterceptRequest) (*pb.InterceptResponse, error)
}
