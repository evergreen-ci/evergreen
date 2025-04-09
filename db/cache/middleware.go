package cache

import (
	"net/http"

	"github.com/evergreen-ci/gimlet"
)

// gimletMiddleware embeds a db cache into the context of the request.
type gimletMiddleware struct {
	name string
}

func NewGimletMiddleware(name string) gimlet.Middleware {
	return &gimletMiddleware{name: name}

}

func (e *gimletMiddleware) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	next.ServeHTTP(rw, r.WithContext(Embed(r.Context(), e.name)))
}
