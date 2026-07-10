package service

import (
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

	sampledRouteWindow = time.Minute
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
	start time.Time
	count int
	paths map[string]struct{}
}

// sampledRequestLogger wraps gimlet's recovery logger. Requests matching
// sampledRouteSuffixes are counted and summarized once per sampledRouteWindow
// instead of logged individually; every other request falls back to
// gimlet's normal per-request logging and panic recovery, unchanged.
type sampledRequestLogger struct {
	fallback gimlet.Middleware

	mu      sync.Mutex
	windows map[string]*routeWindow
}

func newSampledRequestLogger() gimlet.Middleware {
	return &sampledRequestLogger{
		fallback: gimlet.MakeRecoveryLogger(),
		windows:  map[string]*routeWindow{},
	}
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

	l.recordSuccess(r, key, status)
}

// recordSuccess counts a successful call to a sampled route and flushes a
// summary log line within one sample window.
func (l *sampledRequestLogger) recordSuccess(r *http.Request, key string, status int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.windows[key]
	if !ok {
		w = &routeWindow{start: time.Now(), paths: map[string]struct{}{}}
		l.windows[key] = w
	}
	w.count++
	w.paths[r.URL.Path] = struct{}{}

	if time.Since(w.start) < sampledRouteWindow {
		return
	}

	paths := make([]string, 0, len(w.paths))
	for path := range w.paths {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	grip.Info(r.Context(), message.Fields{
		"action":        "completed",
		"route":         key,
		"status":        status,
		"count":         w.count,
		"window_secs":   time.Since(w.start).Seconds(),
		"sampled_paths": paths,
	})

	w.start = time.Now()
	w.count = 0
	w.paths = map[string]struct{}{}
}
