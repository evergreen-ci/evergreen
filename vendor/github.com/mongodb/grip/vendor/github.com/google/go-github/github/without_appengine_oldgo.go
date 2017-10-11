// +build !appengine
// +build !go1.7

// This file provides glue for making github work on old versions of go

package github

import (
	"net/http"

	"context"
)

func withContext(ctx context.Context, req *http.Request) (context.Context, *http.Request) {
	return ctx, req
}
