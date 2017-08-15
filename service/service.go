package service

import (
	"net"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/tylerb/graceful"
)

// RunGracefully borrows extensively from the grace
func RunGracefully(addr string, timeout time.Duration, n http.Handler) error {
	startedAt := time.Now()
	srv := &graceful.Server{
		Timeout:      timeout,
		TCPKeepAlive: time.Minute,
		Server: &http.Server{
			Addr:         addr,
			Handler:      n,
			ReadTimeout:  time.Minute,
			WriteTimeout: time.Minute,
		},
		ShutdownInitiated: func() {
			grip.Notice(message.Fields{
				"uptime":   time.Since(startedAt).String(),
				"action":   "starting graceful shutdown",
				"service":  addr,
				"duration": time.Since(startedAt),
				"build":    evergreen.BuildRevision,
				"process":  grip.Name(),
			})
		},
	}

	grip.Notice(message.Fields{
		"action":  "starting service",
		"service": addr,
		"build":   evergreen.BuildRevision,
		"process": grip.Name(),
	})

	if err := srv.ListenAndServe(); err != nil {
		if opErr, ok := err.(*net.OpError); !ok || (ok && opErr.Op != "accept") {
			return err
		}
	}

	return nil
}
