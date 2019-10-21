package aviation

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockServerStream struct {
	ctx     context.Context
	sentMsg string
	recvMsg string
}

func (_ *mockServerStream) SetHeader(_ metadata.MD) error  { return nil }
func (_ *mockServerStream) SendHeader(_ metadata.MD) error { return nil }
func (_ *mockServerStream) SetTrailer(_ metadata.MD)       {}
func (s *mockServerStream) Context() context.Context       { return s.ctx }
func (_ *mockServerStream) RecvMsg(_ interface{}) error    { return nil }
func (s *mockServerStream) SendMsg(m interface{}) error {
	switch i := m.(type) {
	case string:
		s.sentMsg = i
	case []byte:
		s.sentMsg = string(i)
	}

	return nil
}

func mockUnaryHandler(_ context.Context, req interface{}) (interface{}, error) {
	switch t := req.(type) {
	case string:
		if t == "panic" {
			panic("test panic")
		}
		if t == "error" {
			return nil, errors.New("mock error")
		}
	}
	return nil, nil
}

func mockStreamHandler(srv interface{}, stream grpc.ServerStream) error {
	switch t := srv.(type) {
	case string:
		if t == "panic" {
			panic("test panic")
		}
		if t == "error" {
			return errors.New("mock error")
		}
	}
	return nil
}

func requireContextValue(t *testing.T, ctx context.Context, key string, expectedVal interface{}, msg ...interface{}) {
	val := ctx.Value(key)
	require.NotNil(t, val, msg...)
	require.Equal(t, expectedVal, val, msg...)
}

type mockClientOptions struct {
	code       codes.Code
	errorUntil int
	attempts   int
}

func mockUnaryInvokerFactory(opts *mockClientOptions) grpc.UnaryInvoker {
	return func(ctx context.Context, _ string, _, _ interface{}, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
		opts.attempts += 1

		if opts.errorUntil > 0 && opts.code != codes.OK {
			opts.errorUntil -= 1
			return status.Error(opts.code, "mock error!")
		}

		return nil
	}
}

func mockStreamerFactory(opts *mockClientOptions) grpc.Streamer {
	return func(ctx context.Context, _ *grpc.StreamDesc, _ *grpc.ClientConn, method string, _ ...grpc.CallOption) (grpc.ClientStream, error) {
		opts.attempts += 1

		if opts.errorUntil > 0 && opts.code != codes.OK {
			opts.errorUntil -= 1
			return nil, status.Error(opts.code, "mock error!")
		}

		return nil, nil
	}
}
