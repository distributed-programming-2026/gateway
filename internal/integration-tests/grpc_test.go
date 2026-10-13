package integration_tests

import (
	"context"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pb "github.com/distributed-programming-2026/gateway/api/client/interceptorgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type echoServer interface{}

func echoStream(_ any, stream grpc.ServerStream) error {
	stream.SetTrailer(metadata.Pairs("x-trailer", "preserved"))
	for {
		request := &pb.HttpBody{}
		err := stream.RecvMsg(request)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if err = stream.SendMsg(request); err != nil {
			return err
		}
	}
}
func TestGRPCUnaryStreamingStatusesAndTrailers(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	backend := grpc.NewServer()
	pb.RegisterInterceptorServer(backend, &grpcInterceptor{f: func(ctx context.Context, r *pb.InterceptRequest) (*pb.InterceptResponse, error) {
		m, _ := metadata.FromIncomingContext(ctx)
		assert.Equal(t, "metadata", m.Get("x-test")[0],
			"metadata lost")

		_ = grpc.SetTrailer(ctx, metadata.Pairs("x-trailer", "unary"))
		if r.Context.RequestId == "deny" {
			return nil, status.Error(codes.PermissionDenied, "forbidden")
		}
		return ok(&pb.ContinueResponse{HeaderMutation: set("X-Echo", r.Context.RequestId)}), nil
	}})
	backend.RegisterService(&grpc.ServiceDesc{ServiceName: "test.Echo", HandlerType: (*echoServer)(nil), Streams: []grpc.StreamDesc{{StreamName: "Chat", Handler: echoStream, ServerStreams: true, ClientStreams: true}}}, struct{}{})
	go func() { _ = backend.Serve(listener) }()
	defer backend.Stop()
	gateway := gateway(t, fmt.Sprintf(`kind: gateway/service/v1
name: grpc
routes:
  - pattern: '^/(interceptorgrpc\.Interceptor|test\.Echo)/.*$'
    target: '%s'
    useHTTP2: true
    timeout: 0s
`, listener.Addr()))
	conn, err := grpc.NewClient(gateway.Listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)

	defer func() { _ = conn.Close() }()
	ctx, cancel := context.WithTimeout(metadata.NewOutgoingContext(context.Background(), metadata.Pairs("x-test", "metadata")), 3*time.Second)
	defer cancel()
	client := pb.NewInterceptorClient(conn)
	var trailer metadata.MD
	response, err := client.Intercept(ctx, &pb.InterceptRequest{Context: &pb.ProcessingContext{RequestId: "echo"}}, grpc.Trailer(&trailer))
	require.NoError(t, err)
	require.Equal(t, "echo", response.GetContinueResponse().HeaderMutation.Set[0].Values[0],
		"unary response or trailers lost")
	require.Equal(t, "unary", trailer.Get("x-trailer")[0],
		"unary response or trailers lost")

	_, err = client.Intercept(ctx, &pb.InterceptRequest{Context: &pb.ProcessingContext{RequestId: "deny"}})
	require.Equal(t, codes.PermissionDenied, status.Code(err),
		"gRPC status lost %v", err)

	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true, ClientStreams: true}, "/test.Echo/Chat")
	require.NoError(t, err)

	for _, body := range []string{"first", "second\x00"} {
		err = stream.SendMsg(&pb.HttpBody{Data: []byte(body)})
		require.NoError(t, err)

		reply := &pb.HttpBody{}
		err = stream.RecvMsg(reply)
		require.NoError(t, err)
		require.Equal(t, body, string(reply.Data),
			"stream message corrupted")
	}
	err = stream.CloseSend()
	require.NoError(t, err)
	err = stream.RecvMsg(&pb.HttpBody{})
	require.Equal(t, io.EOF, err)
	require.Equal(t, "preserved", stream.Trailer().Get("x-trailer")[0],
		"stream trailers lost")
}
