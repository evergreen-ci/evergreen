// Main package for the Evergreen runner.
package main

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	. "github.com/evergreen-ci/evergreen/runner"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"
)

var (
	runInterval = int64(30)
)

func main() {
	settings := evergreen.MustConfig()
	if settings.Runner.LogFile != "" {
		evergreen.SetLogger(settings.Runner.LogFile)
	}

	if settings.Runner.IntervalSeconds <= 0 {
		evergreen.Logger.Logf(slogger.WARN, "Interval set to %vs (<= 0s) using %vs instead", settings.Runner.IntervalSeconds, runInterval)
	} else {
		runInterval = settings.Runner.IntervalSeconds
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	// start and schedule runners
	wg := &sync.WaitGroup{}
	ch := startRunners(wg, settings)
	go listenForSIGTERM(ch)

	// wait for all the processes to exit
	wg.Wait()
}

// runProcess runs a ProcessRunner with the given settings.
func runProcess(r ProcessRunner, s *evergreen.Settings) {
	if err := r.Run(s); err != nil {
		evergreen.Logger.Logf(slogger.WARN, "error running %v: %v", r.Name(), err)
	}
}

// startRunners starts a goroutine for each runner exposed via Runners. It
// returns a channel on which all runners listen on, for when to terminate.
func startRunners(wg *sync.WaitGroup, s *evergreen.Settings) chan struct{} {
	c := make(chan struct{}, 1)
	for _, r := range Runners {
		wg.Add(1)

		// start each runner in its own goroutine
		go func(r ProcessRunner, s *evergreen.Settings, c chan struct{}) {
			defer wg.Done()
			ticker := time.NewTicker(time.Duration(runInterval) * time.Second)
			evergreen.Logger.Logf(slogger.INFO, "Starting %v", r.Name())
			// start the runner immediately
			runProcess(r, s)

			loop := true
			for loop {
				select {
				case <-c:
					loop = false
				case <-ticker.C:
					runProcess(r, s)
				}
			}
			evergreen.Logger.Logf(slogger.INFO, "Cleanly terminated %v", r.Name())
		}(r, s, c)
	}
	return c
}

// listenForSIGTERM listens for the SIGTERM signal and closes the
// channel on which each runner is listening as soon as the signal
// is received.
func listenForSIGTERM(ch chan struct{}) {
	sigChan := make(chan os.Signal)
	// notify us when SIGTERM is received
	signal.Notify(sigChan, syscall.SIGTERM)
	<-sigChan
	evergreen.Logger.Logf(slogger.INFO, "Terminating %v processes", len(Runners))
	close(ch)
}
