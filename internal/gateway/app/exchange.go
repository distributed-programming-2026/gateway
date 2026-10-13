package app

import (
	"context" // SHA htpasswd compatibility, not password generation.
	"errors"
	"net/http"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"github.com/distributed-programming-2026/gateway/internal/gateway/config"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type exchange struct {
	request          *http.Request
	route            *config.Route
	id               string
	pending          []*pb.HeaderMutation
	failed           map[string]bool
	gatewayError     *pb.GatewayError
	requestReplaced  bool
	responseReplaced bool
}
type processingError struct {
	status      int
	code        string
	interceptor string
}

func (e *processingError) Error() string { return e.code }
func failure(code string, statusCode int) *processingError {
	return &processingError{code: code, status: statusCode}
}
func timeout(err error) bool {
	var timed interface{ Timeout() bool }
	return errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded || (errors.As(err, &timed) && timed.Timeout())
}

const interceptorError = "interceptor_error"
