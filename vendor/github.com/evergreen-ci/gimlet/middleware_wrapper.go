package gimlet

import "net/http"

// WrapperMiddleware is convenience function to produce middlewares
// from functions that wrap http.HandlerFuncs.
func WrapperMiddleware(w HandlerWrapper) Middleware { return w }

// HandlerWrapper provides a way to define a middleware as a function
// rather than a type.
type HandlerWrapper func(http.HandlerFunc) http.HandlerFunc

// ServeHTTP provides a gimlet.Middleware compatible shim for
// HandlerWrapper-typed middlewares.
func (w HandlerWrapper) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w(next)(rw, r)
}
