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

	sampledFlushInterval = 30 * time.Second
)

// sampledRoute describes a route that gets its requests counted and
// summarized instead of logged individually. idField names the log field
// under which the route's path variable (e.g. task ID, host ID) is reported.
type sampledRoute struct {
	suffix  string
	idField string
}

var sampledRoutes = []sampledRoute{
	{suffix: nextTaskRouteSuffix, idField: "host_ids"},
	{suffix: heartbeatRouteSuffix, idField: "task_ids"},
}

func matchSampledRoute(path string) (sampledRoute, bool) {
	for _, route := range sampledRoutes {
		if strings.HasSuffix(path, route.suffix) {
			return route, true
		}
	}
	return sampledRoute{}, false
}

// routeIDFromPath returns the path segment immediately preceding suffix,
// e.g. the task ID in ".../task/{task_id}/heartbeat".
func routeIDFromPath(path, suffix string) string {
	trimmed := strings.TrimSuffix(path, suffix)
	return trimmed[strings.LastIndex(trimmed, "/")+1:]
}

type routeWindow struct {
	count   int
	status  int
	idField string
	ids     map[string]struct{}
}

// sampledRequestLogger wraps gimlet's recovery logger. Requests matching
// sampledRoutes are counted and summarized once per flush interval instead
// of logged individually.
type sampledRequestLogger struct {
	fallback gimlet.Middleware

	mu      sync.Mutex
	windows map[string]*routeWindow
}

// newSampledRequestLogger starts a background goroutine that flushes each
// sampled route's summary every sampledFlushInterval.
func newSampledRequestLogger(ctx context.Context) gimlet.Middleware {
	l := &sampledRequestLogger{
		fallback: gimlet.MakeRecoveryLogger(),
		windows:  map[string]*routeWindow{},
	}

	ticker := time.NewTicker(sampledFlushInterval)
	go func() {
		for range ticker.C {
			l.flush(ctx)
		}
	}()

	return l
}

func (l *sampledRequestLogger) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	route, ok := matchSampledRoute(r.URL.Path)
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
		fields := message.Fields{
			"action": "completed",
			"method": r.Method,
			"path":   r.URL.Path,
			"remote": r.RemoteAddr,
			"status": status,
		}
		if status == http.StatusConflict && route.suffix == heartbeatRouteSuffix {
			// A task's secret is expected to no longer match after the task
			// is aborted and restarted to a new execution, so this is not an
			// error.
			grip.Info(r.Context(), fields)
		} else {
			grip.Error(r.Context(), fields)
		}
		return
	}

	l.recordSuccess(route, r.URL.Path, status)
}

// recordSuccess counts a successful call to a sampled route.
func (l *sampledRequestLogger) recordSuccess(route sampledRoute, path string, status int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	w, ok := l.windows[route.suffix]
	if !ok {
		w = &routeWindow{idField: route.idField, ids: map[string]struct{}{}}
		l.windows[route.suffix] = w
	}
	w.count++
	w.status = status
	w.ids[routeIDFromPath(path, route.suffix)] = struct{}{}
}

// flush logs and resets every route's accumulated count and IDs.
func (l *sampledRequestLogger) flush(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for key, w := range l.windows {
		if w.count == 0 {
			continue
		}

		ids := make([]string, 0, len(w.ids))
		for id := range w.ids {
			ids = append(ids, id)
		}
		sort.Strings(ids)

		grip.Info(ctx, message.Fields{
			"action":  "completed",
			"route":   key,
			"status":  w.status,
			"count":   w.count,
			w.idField: ids,
		})

		w.count = 0
		w.ids = map[string]struct{}{}
	}
}
