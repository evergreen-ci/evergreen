package service

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/negroni"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const (
	heartbeatRouteSuffix = "/heartbeat"
	nextTaskRouteSuffix  = "/agent/next_task"
)

var sampledRouteSuffixes = []string{
	nextTaskRouteSuffix,
	heartbeatRouteSuffix,
}

func sampledRouteKey(path string) (string, bool) {
	for _, suffix := range sampledRouteSuffixes {
		if strings.HasSuffix(path, suffix) {
			return suffix, true
		}
	}
	return "", false
}

type routeWindow struct {
	count  int
	status int
	paths  map[string]struct{}
}

// sampledRequestLogger wraps gimlet's recovery logger. Requests matching
// sampledRouteSuffixes are counted and summarized once per minute
// instead of logged individually; every other request falls back to
// gimlet's normal per-request logging and panic recovery, unchanged.
type sampledRequestLogger struct {
	fallback gimlet.Middleware

	mu      sync.Mutex
	windows map[string]*routeWindow
}

// newSampledRequestLogger starts a background goroutine that flushes each
// sampled route's summary every minute.
func newSampledRequestLogger(ctx context.Context) gimlet.Middleware {
	l := &sampledRequestLogger{
		fallback: gimlet.MakeRecoveryLogger(),
		windows:  map[string]*routeWindow{},
	}

	ticker := time.NewTicker(time.Minute)
	go func() {
		for range ticker.C {
			l.flush(ctx)
		}
	}()

	return l
}

func (l *sampledRequestLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	key, ok := sampledRouteKey(r.URL.Path)
	if !ok {
		l.fallback.ServeHTTP(rw, r, next)
		return
	}

	defer func() {
		if p := recover(); p != nil {
			grip.Error(r.Context(), message.Fields{
				"action": "aborted",
				"path":   r.URL.Path,
				"remote": r.RemoteAddr,
				"panic":  fmt.Sprintf("%v", p),
				"stack":  string(debug.Stack()),
			})
			gimlet.WriteJSONInternalError(r.Context(), rw, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "request aborted",
			})
		}
	}()

	next(rw, r)

	// Log as normal if route failed.
	status := rw.(negroni.ResponseWriter).Status()
	if status != http.StatusOK {
		grip.Error(r.Context(), message.Fields{
			"action": "completed",
			"method": r.Method,
			"path":   r.URL.Path,
			"remote": r.RemoteAddr,
			"status": status,
		})
		return
	}

	l.recordSuccess(key, r.URL.Path, status)
}

// recordSuccess counts a successful call to a sampled route.
func (l *sampledRequestLogger) recordSuccess(key, path string, status int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.windows[key]
	if !ok {
		w = &routeWindow{paths: map[string]struct{}{}}
		l.windows[key] = w
	}
	w.count++
	w.status = status
	w.paths[path] = struct{}{}
}

// flush logs and resets every route's accumulated count and paths.
func (l *sampledRequestLogger) flush(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, w := range l.windows {
		if w.count == 0 {
			continue
		}

		paths := make([]string, 0, len(w.paths))
		for path := range w.paths {
			paths = append(paths, path)
		}
		sort.Strings(paths)

		grip.Info(ctx, message.Fields{
			"action":        "completed",
			"route":         key,
			"status":        w.status,
			"count":         w.count,
			"sampled_paths": paths,
		})

		w.count = 0
		w.paths = map[string]struct{}{}
	}
}
