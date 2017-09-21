// +build !go1.7

package gimlet

import (
	"net/http"

	"golang.org/x/net/context"
)

func getRequestContext(r *http.Request) (*http.Request, context.Context) {
	return r, context.Background()
}
