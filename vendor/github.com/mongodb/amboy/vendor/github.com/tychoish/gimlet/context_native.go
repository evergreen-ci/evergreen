// +build go1.7

package gimlet

import (
	"context"
	"net/http"
)

func getRequestContext(r *http.Request) (*http.Request, context.Context) { return r, r.Context() }
