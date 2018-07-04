package gimlet

import "net/http"

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
