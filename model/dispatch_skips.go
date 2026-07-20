package model

import "context"

type dispatchSkipsKey struct{}

// NewDispatchSkipsContext returns a context that counts the number of tasks
// skipped in a next_task attempt.
func NewDispatchSkipsContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, dispatchSkipsKey{}, new(int))
}

// RecordDispatchSkip increments the context's dispatch skip counter.
func RecordDispatchSkip(ctx context.Context) {
	if count, ok := ctx.Value(dispatchSkipsKey{}).(*int); ok {
		*count++
	}
}

// DispatchSkipsFromContext returns the number of dispatch skips recorded in
// the context.
func DispatchSkipsFromContext(ctx context.Context) int {
	if count, ok := ctx.Value(dispatchSkipsKey{}).(*int); ok {
		return *count
	}
	return 0
}
