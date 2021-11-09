package aviation

import (
	"context"
)

var jobIDSource <-chan int

func init() {
	jobIDSource = func() <-chan int {
		out := make(chan int, 50)
		go func() {
			var jobID int
			for {
				jobID++
				out <- jobID
			}
		}()
		return out
	}()
}

// getNumber is a source of safe monotonically increasing integers
// for use in request ids.
func getNumber() int {
	return <-jobIDSource
}

// SetRequesID attaches a request id to a context.
func SetRequestID(ctx context.Context, id int) context.Context {
	return context.WithValue(ctx, requestIDKey, id)
}

// GetRequestID returns the unique (monotonically increaseing) ID of
// the request since startup
func GetRequestID(ctx context.Context) int {
	if rv := ctx.Value(requestIDKey); rv != nil {
		if id, ok := rv.(int); ok {
			return id
		}
	}

	return -1
}
