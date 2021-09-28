package gimlet

import (
	"github.com/urfave/negroni"
)

// Middleware is a local alias for negroni.Handler types.
type Middleware negroni.Handler

type contextKey int

const (
	requestIDKey contextKey = iota
	loggerKey
	startAtKey
	authHandlerKey
	userManagerKey
	userKey
	loggingAnnotationsKey
)
