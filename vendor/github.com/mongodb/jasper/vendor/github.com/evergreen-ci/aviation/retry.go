package aviation

import (
	"context"
	"time"

	"github.com/jpillora/backoff"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
)

func MakeRetryUnaryClientInterceptor(maxRetries int) grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		var lastErr error

		b := getBackoff(100 * time.Millisecond)

		var i int
		start := time.Now()
	retry:
		for i = 0; i < maxRetries; i++ {
			lastErr = invoker(ctx, method, req, reply, cc, opts...)
			grip.Info(message.Fields{
				"message": "GRPC client retry",
				"method":  method,
				"attempt": i,
				"error":   lastErr,
			})

			if lastErr == nil {
				return nil
			}

			if !isRetriable(lastErr) {
				break retry
			}

			timer := time.NewTimer(b.Duration())
			select {
			case <-ctx.Done():
				timer.Stop()
				break retry
			case <-timer.C:
				timer.Stop()
				continue retry
			}
		}

		timeElapsed := time.Now().Sub(start)
		grip.Warning(message.WrapError(lastErr, message.Fields{
			"message":       "GRPC client retries exceeded or canceled",
			"method":        method,
			"total_retries": i,
			"time_elasped":  timeElapsed,
		}))
		return lastErr
	}
}

func MakeRetryStreamClientInterceptor(maxRetries int) grpc.StreamClientInterceptor {
	return func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		var clientStream grpc.ClientStream
		var lastErr error

		b := getBackoff(100 * time.Millisecond)

		var i int
		start := time.Now()
	retry:
		for i = 0; i < maxRetries; i++ {
			clientStream, lastErr = streamer(ctx, desc, cc, method, opts...)
			grip.Info(message.Fields{
				"message": "GRPC client retry",
				"method":  method,
				"attempt": i,
				"error":   lastErr,
			})

			if lastErr == nil {
				return clientStream, nil
			}

			if !isRetriable(lastErr) {
				break retry
			}

			timer := time.NewTimer(b.Duration())
			select {
			case <-ctx.Done():
				timer.Stop()
				break retry
			case <-timer.C:
				timer.Stop()
				continue retry
			}
		}

		timeElapsed := time.Now().Sub(start)
		grip.Warning(message.WrapError(lastErr, message.Fields{
			"message":       "GRPC client retries exceeded or canceled",
			"method":        method,
			"total_retries": i,
			"time_elasped":  timeElapsed,
		}))
		return nil, lastErr
	}
}

func getBackoff(min time.Duration) *backoff.Backoff {
	return &backoff.Backoff{
		Min:    min,
		Max:    min * 10,
		Factor: 2,
		Jitter: false,
	}
}

func isRetriable(err error) bool {
	switch errCode := grpc.Code(err); errCode {
	case codes.Unknown:
		return true
	case codes.ResourceExhausted:
		return true
	case codes.Aborted:
		return true
	case codes.Internal:
		return true
	case codes.Unavailable:
		return true
	default:
		return false
	}
}
