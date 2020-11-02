package aviation

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
)

func TestMakeUnaryClientInterceptor(t *testing.T) {
	for _, test := range []struct {
		name             string
		ctxTimeout       time.Duration
		grpcCodes        []codes.Code
		errorUntil       int
		retries          int
		expectedAttempts int
		hasErr           bool
		expectedMinTime  time.Duration
		expectedMaxTime  time.Duration
	}{
		{
			name: "NoRetryFail",
			grpcCodes: []codes.Code{
				codes.Canceled,
				codes.InvalidArgument,
				codes.DeadlineExceeded,
				codes.NotFound,
				codes.AlreadyExists,
				codes.PermissionDenied,
				codes.FailedPrecondition,
				codes.OutOfRange,
				codes.Unimplemented,
				codes.DataLoss,
				codes.Unauthenticated,
			},
			errorUntil:       1,
			retries:          10,
			expectedAttempts: 1,
			hasErr:           true,
			expectedMaxTime:  100 * time.Millisecond,
		},
		{
			name:             "NoRetrySuccess",
			grpcCodes:        []codes.Code{codes.OK},
			errorUntil:       0,
			retries:          10,
			expectedAttempts: 1,
			expectedMaxTime:  100 * time.Millisecond,
		},
		{
			name: "RetryFail",
			grpcCodes: []codes.Code{
				codes.Unknown,
				codes.ResourceExhausted,
				codes.Aborted,
				codes.Internal,
				codes.Unavailable,
			},
			errorUntil:       3,
			retries:          3,
			expectedAttempts: 3,
			hasErr:           true,
			expectedMinTime:  300 * time.Millisecond,
			expectedMaxTime:  1000 * time.Millisecond,
		},
		{
			name: "RetrySucess",
			grpcCodes: []codes.Code{
				codes.Unknown,
				codes.ResourceExhausted,
				codes.Aborted,
				codes.Internal,
				codes.Unavailable,
			},
			errorUntil:       3,
			retries:          10,
			expectedAttempts: 4,
			expectedMinTime:  700 * time.Millisecond,
			expectedMaxTime:  1000 * time.Millisecond,
		},
		{
			name:             "ParentContextInterrupt",
			ctxTimeout:       150 * time.Millisecond,
			grpcCodes:        []codes.Code{codes.Unknown},
			errorUntil:       3,
			retries:          10,
			expectedAttempts: 2,
			hasErr:           true,
			expectedMinTime:  100 * time.Millisecond,
			expectedMaxTime:  200 * time.Millisecond,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, code := range test.grpcCodes {
				// unary
				opts := &mockClientOptions{
					code:       code,
					errorUntil: test.errorUntil,
				}
				invoker := mockUnaryInvokerFactory(opts)
				interceptor := MakeRetryUnaryClientInterceptor(test.retries)
				ctx := context.Background()
				if test.ctxTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, test.ctxTimeout)
					defer cancel()
				}

				start := time.Now()
				err := interceptor(ctx, "", nil, nil, nil, invoker)
				end := time.Now()
				actualTime := end.Sub(start)

				if test.hasErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				assert.Equal(t, test.expectedAttempts, opts.attempts)
				assert.True(t, test.expectedMinTime <= actualTime)
				assert.True(t, test.expectedMaxTime >= actualTime)

				// stream
				opts = &mockClientOptions{
					code:       code,
					errorUntil: test.errorUntil,
				}
				streamer := mockStreamerFactory(opts)
				streamInterceptor := MakeRetryStreamClientInterceptor(test.retries)
				if test.ctxTimeout > 0 {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(context.Background(), test.ctxTimeout)
					defer cancel()
				}

				start = time.Now()
				_, err = streamInterceptor(ctx, nil, nil, "", streamer)
				end = time.Now()
				actualTime = end.Sub(start)

				if test.hasErr {
					assert.Error(t, err)
				} else {
					assert.NoError(t, err)
				}
				assert.Equal(t, test.expectedAttempts, opts.attempts)
				assert.True(t, test.expectedMinTime <= actualTime)
				assert.True(t, test.expectedMaxTime >= actualTime)

			}
		})
	}
}
