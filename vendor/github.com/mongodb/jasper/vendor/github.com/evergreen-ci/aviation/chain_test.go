package aviation

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	someServiceName  = "SomeService.StreamMethod"
	parentUnaryInfo  = &grpc.UnaryServerInfo{FullMethod: someServiceName}
	parentStreamInfo = &grpc.StreamServerInfo{
		FullMethod:     someServiceName,
		IsServerStream: true,
	}
	someValue     = 1
	parentContext = context.WithValue(context.TODO(), "parent", someValue)
)

func TestChainUnaryServer(t *testing.T) {
	input := "input"
	output := "output"

	first := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(t, ctx, "parent", someValue, "first interceptor must know the parent context value")
		require.Equal(t, parentUnaryInfo, info, "first interceptor must know the someUnaryServerInfo")
		ctx = context.WithValue(ctx, "first", 1)
		return handler(ctx, req)
	}
	second := func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		requireContextValue(t, ctx, "parent", someValue, "second interceptor must know the parent context value")
		requireContextValue(t, ctx, "first", someValue, "second interceptor must know the first context value")
		require.Equal(t, parentUnaryInfo, info, "second interceptor must know the someUnaryServerInfo")
		ctx = context.WithValue(ctx, "second", someValue)
		return handler(ctx, req)
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		require.EqualValues(t, input, req, "handler must get the input")
		requireContextValue(t, ctx, "parent", someValue, "handler must know the parent context value")
		requireContextValue(t, ctx, "first", someValue, "handler must know the first context value")
		requireContextValue(t, ctx, "second", someValue, "handler must know the second context value")
		return output, nil
	}

	chain := ChainUnaryServer(first, second)
	out, _ := chain(parentContext, input, parentUnaryInfo, handler)
	require.EqualValues(t, output, out, "chain must return handler's output")
}

func TestChainStreamServer(t *testing.T) {
	someService := &struct{}{}
	recvMsg := "received"
	sentMsg := "sent"
	outputError := fmt.Errorf("some error")

	first := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		requireContextValue(t, stream.Context(), "parent", someValue, "first interceptor must know the parent context value")
		require.Equal(t, parentStreamInfo, info, "first interceptor must know the parentStreamInfo")
		require.Equal(t, someService, srv, "first interceptor must know someService")
		raw := stream.(*mockServerStream)
		raw.ctx = context.WithValue(raw.ctx, "first", someValue)
		return handler(srv, stream)
	}
	second := func(srv interface{}, stream grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		requireContextValue(t, stream.Context(), "parent", someValue, "second interceptor must know the parent context value")
		requireContextValue(t, stream.Context(), "parent", someValue, "second interceptor must know the first context value")
		require.Equal(t, parentStreamInfo, info, "second interceptor must know the parentStreamInfo")
		require.Equal(t, someService, srv, "second interceptor must know someService")
		raw := stream.(*mockServerStream)
		raw.ctx = context.WithValue(raw.ctx, "second", someValue)
		return handler(srv, stream)
	}
	handler := func(srv interface{}, stream grpc.ServerStream) error {
		require.Equal(t, someService, srv, "handler must know someService")
		requireContextValue(t, stream.Context(), "parent", someValue, "handler must know the parent context value")
		requireContextValue(t, stream.Context(), "first", someValue, "handler must know the first context value")
		requireContextValue(t, stream.Context(), "second", someValue, "handler must know the second context value")
		require.NoError(t, stream.RecvMsg(recvMsg), "handler must have access to stream messages")
		require.NoError(t, stream.SendMsg(sentMsg), "handler must be able to send stream messages")
		return outputError
	}
	fakeStream := &mockServerStream{ctx: parentContext, recvMsg: recvMsg}
	chain := ChainStreamServer(first, second)
	err := chain(someService, fakeStream, parentStreamInfo, handler)
	require.Equal(t, outputError, err, "chain must return handler's error")
	require.Equal(t, sentMsg, fakeStream.sentMsg, "handler's sent message must propagate to stream")
}
