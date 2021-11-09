package aviation

import (
	"context"
	"time"

	"github.com/evergreen-ci/gimlet"
)

type contextKey int

const (
	requestIDKey contextKey = iota
	startAtKey
	userKey
)

func SetRequestStartAt(ctx context.Context, startAt time.Time) context.Context {
	return context.WithValue(ctx, startAtKey, startAt)
}

func GetRequestStartAt(ctx context.Context) time.Time {
	if rv := ctx.Value(startAtKey); rv != nil {
		if t, ok := rv.(time.Time); ok {
			return t
		}
	}

	return time.Time{}
}

// SetRequestUser adds a user to a context. This function is public to
// support teasing workflows.
func SetRequestUser(ctx context.Context, u gimlet.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// GetUser returns the user attached to the request. The User object
// is nil when
func GetUser(ctx context.Context) gimlet.User {
	u := ctx.Value(userKey)
	if u == nil {
		return nil
	}

	usr, ok := u.(gimlet.User)
	if !ok {
		return nil
	}

	return usr
}
