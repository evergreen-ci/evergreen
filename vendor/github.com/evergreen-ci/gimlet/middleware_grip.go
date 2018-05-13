package gimlet

import (
	"context"
	"net/http"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/urfave/negroni"
)

const (
	remoteAddrHeaderName = "X-Cluster-Client-Ip"
)

// appLogging provides a Negroni-compatible middleware to send all
// logging using the grip packages logging. This defaults to using
// systemd logging, but gracefully falls back to use go standard
// library logging, with some additional helpers and configurations to
// support configurable level-based logging. This particular
// middlewear resembles the basic tracking provided by Negroni's
// standard logging system.
type appLogging struct {
	grip.Journaler
}

// NewAppLogger creates an logging middlear instance suitable for use
// with Negroni. Sets the logging configuration to be the same as the
// default global grip logging object.
func NewAppLogger() Middleware { return &appLogging{logging.MakeGrip(grip.GetSender())} }

func setServiceLogger(r *http.Request, logger grip.Journaler) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), loggerKey, logger))
}

func setStartAtTime(r *http.Request, startAt time.Time) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), startAtKey, startAt))
}

func getRequestStartAt(ctx context.Context) time.Time {
	if rv := ctx.Value(startAtKey); rv != nil {
		if t, ok := rv.(time.Time); ok {
			return t
		}
	}

	return time.Time{}
}

func GetLogger(ctx context.Context) grip.Journaler {
	if rv := ctx.Value(loggerKey); rv != nil {
		if l, ok := rv.(grip.Journaler); ok {
			return l
		}
	}

	return logging.MakeGrip(grip.GetSender())
}

// Logs the request path, the beginning of every request as well as
// the duration upon completion and the status of the response.
func (l *appLogging) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	r = setupLogger(l.Journaler, r)

	next(rw, r)
	res := rw.(negroni.ResponseWriter)
	finishLogger(l.Journaler, r, res)
}

func setupLogger(logger grip.Journaler, r *http.Request) *http.Request {
	r = setServiceLogger(r, logger)
	remote := r.Header.Get(remoteAddrHeaderName)
	if remote == "" {
		r.RemoteAddr = remote
	}

	id := getNumber()
	r = setRequestID(r, id)
	startAt := time.Now()
	r = setStartAtTime(r, startAt)

	logger.Info(message.Fields{
		"action":  "started",
		"method":  r.Method,
		"remote":  r.RemoteAddr,
		"request": id,
		"path":    r.URL.Path,
	})

	return r
}

func finishLogger(logger grip.Journaler, r *http.Request, res negroni.ResponseWriter) {
	ctx := r.Context()
	startAt := getRequestStartAt(ctx)
	dur := time.Since(startAt)

	logger.Info(message.Fields{
		"method":      r.Method,
		"remote":      r.RemoteAddr,
		"request":     GetRequestID(ctx),
		"path":        r.URL.Path,
		"duration_ms": int64(dur / time.Millisecond),
		"action":      "completed",
		"status":      res.Status(),
		"outcome":     http.StatusText(res.Status()),
	})
}

// This is largely duplicated from the above, but lets us optionally
type appRecoveryLogger struct {
	grip.Journaler
}

func NewRecoveryLogger() Middleware { return &appRecoveryLogger{} }

func (l *appRecoveryLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	r = setupLogger(l.Journaler, r)
	ctx := r.Context()

	defer func() {
		if err := recover(); err != nil {
			if rw.Header().Get("Content-Type") == "" {
				rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
			}

			rw.WriteHeader(http.StatusInternalServerError)

			l.Critical(message.WrapStack(2, message.Fields{
				"panic":   err,
				"action":  "aborted",
				"request": GetRequestID(ctx),
				"path":    r.URL.Path,
				"remote":  r.RemoteAddr,
			}))
		}
	}()

	next(rw, r)

	res := rw.(negroni.ResponseWriter)
	finishLogger(l.Journaler, r, res)
}
