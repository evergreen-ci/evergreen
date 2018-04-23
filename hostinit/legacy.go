package hostinit

import (
	"context"
	"math/rand"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

func startHosts(ctx context.Context, settings *evergreen.Settings) error {

	startTime := time.Now()

	hostsToStart, err := host.Find(host.IsUninitialized)
	if err != nil {
		return errors.Wrap(err, "error fetching uninitialized hosts")
	}

	startQueue := make([]host.Host, len(hostsToStart))
	for i, r := range rand.Perm(len(hostsToStart)) {
		startQueue[i] = hostsToStart[r]
	}

	catcher := grip.NewBasicCatcher()

	var started int

	for idx := range startQueue {
		if ctx.Err() != nil {
			return errors.New("hostinit run canceled")
		}

		h := &startQueue[idx]

		if h.UserHost {
			// pass:
			//    always start spawn hosts asap
		} else if started > 12 {
			// throttle hosts, so that we're starting very
			// few hosts on every pass. Hostinit runs very
			// frequently, lets not start too many all at
			// once.

			continue
		}

		err = CreateHost(ctx, h, settings)

		if errors.Cause(err) == errIgnorableCreateHost {
			continue
		} else if err != nil {
			catcher.Add(err)
			continue
		}

		started++
	}

	m := message.Fields{
		"runner":     RunnerName,
		"method":     "startHosts",
		"num_hosts":  started,
		"total":      len(hostsToStart),
		"runtime":    time.Since(startTime),
		"had_errors": false,
	}

	if catcher.HasErrors() {
		m["errors"] = catcher.Resolve()
		m["had_errors"] = true
	}

	grip.CriticalWhen(catcher.HasErrors(), m)
	grip.InfoWhen(!catcher.HasErrors(), m)

	return catcher.Resolve()
}

// setupReadyHosts runs the distro setup script of all hosts that are up and reachable.
func setupReadyHosts(ctx context.Context, settings *evergreen.Settings) error {
	// find all hosts in the uninitialized state
	uninitializedHosts, err := host.Find(host.NeedsProvisioning())
	if err != nil {
		return errors.Wrap(err, "error fetching starting hosts")
	}

	grip.Info(message.Fields{
		"message": "uninitialized hosts",
		"number":  len(uninitializedHosts),
		"runner":  RunnerName,
	})

	// used for making sure we don't exit before a setup script is done
	wg := &sync.WaitGroup{}
	catcher := grip.NewSimpleCatcher()
	hosts := make(chan host.Host, len(uninitializedHosts))
	for _, idx := range rand.Perm(len(uninitializedHosts)) {
		hosts <- uninitializedHosts[idx]
	}
	close(hosts)

	numThreads := 24
	if len(uninitializedHosts) < numThreads {
		numThreads = len(uninitializedHosts)
	}

	hostsProvisioned := &util.SafeCounter{}
	startAt := time.Now()
	for i := 0; i < numThreads; i++ {
		wg.Add(1)
		go func() {
			defer recovery.LogStackTraceAndContinue("setupReadyHosts")
			defer wg.Done()
			for {
				select {
				case <-ctx.Done():
					catcher.Add(errors.New("hostinit run canceled"))
					return
				case h, ok := <-hosts:
					if !ok {
						return
					}

					err := SetupHost(ctx, &h, settings)
					if errors.Cause(err) == errRetryHost {
						continue
					}

					catcher.Add(err)
					if err != nil {
						continue
					}

					hostsProvisioned.Inc()
				}
			}
		}()
	}

	// let all setup routines finish
	wg.Wait()
	grip.Info(message.Fields{
		"duration_secs":     time.Since(startAt).Seconds(),
		"runner":            RunnerName,
		"num_hosts":         len(uninitializedHosts),
		"num_errors":        catcher.Len(),
		"provisioned_hosts": hostsProvisioned.Value(),
		"workers":           numThreads,
	})

	return catcher.Resolve()
}
