package gimlet

import (
	"net/http"

	"github.com/urfave/negroni"
)

// WrapperMiddleware is convenience function to produce middlewares
// from functions that wrap http.HandlerFuncs.
func WrapperMiddleware(w HandlerFuncWrapper) Middleware { return w }

// HandlerFuncWrapper provides a way to define a middleware as a function
// rather than a type.
type HandlerFuncWrapper func(http.HandlerFunc) http.HandlerFunc

// ServeHTTP provides a gimlet.Middleware compatible shim for
// HandlerFuncWrapper-typed middlewares.
func (w HandlerFuncWrapper) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w(next)(rw, r)
}

// HandlerWrapper provides a way to define a middleware as a function
// rather than a type for improved interoperability with native
// tools.
type HandlerWrapper func(http.Handler) http.Handler

// ServeHTTP provides a gimlet.Middleware compatible shim for
// HandlerWrapper-typed middlewares.
func (w HandlerWrapper) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	w(next).ServeHTTP(rw, r)
}

// WrapperHandlerMiddleware is convenience function to produce middlewares
// from functions that wrap http.Handlers.
func WrapperHandlerMiddleware(w HandlerWrapper) Middleware { return w }

// MergeMiddleware combines a number of middleware into a single
// middleware instance.
func MergeMiddleware(mws ...Middleware) Middleware {
	if len(mws) == 0 {
		panic("must merge one or more middlewares")
	}

	n := negroni.New()
	for _, m := range mws {
		n.Use(m)
	}

	return negroni.Wrap(n)
}
