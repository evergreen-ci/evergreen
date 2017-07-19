// +build !go1.7

package route

import (
	"net/http"

	"golang.org/x/net/context"
)

func getBaseContext(r *http.Request) context.Context { return context.Background() }
