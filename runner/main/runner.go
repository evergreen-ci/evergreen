// Main package for the Evergreen runner.
package main

import (
	"flag"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/notify"
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
	settings := evergreen.GetSettingsOrExit()
	if settings.Runner.LogFile != "" {
		evergreen.SetLogger(settings.Runner.LogFile)
	}

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	// just run one process if an argument was passed in
	if flag.Arg(0) != "" {
		err := runProcessByName(flag.Arg(0), settings)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error: %v", err)
			os.Exit(1)
		}
		return
	}

	if settings.Runner.IntervalSeconds <= 0 {
		evergreen.Logger.Logf(slogger.WARN, "Interval set to %vs (<= 0s) using %vs instead",
			settings.Runner.IntervalSeconds, runInterval)
	} else {
		runInterval = settings.Runner.IntervalSeconds
	}

	// start and schedule runners
	wg := &sync.WaitGroup{}
	ch := startRunners(wg, settings)
	go listenForSIGTERM(ch)

	// wait for all the processes to exit
	wg.Wait()
}

// runProcess runs a ProcessRunner with the given settings and
// returns a channel on which the caller can listen for process completion.
func runProcess(r ProcessRunner, s *evergreen.Settings) chan struct{} {
	c := make(chan struct{})
	go func(doneChan chan struct{}) {
		err := r.Run(s)
		if err != nil {
			subject := fmt.Sprintf(`%v failure`, r.Name())
			evergreen.Logger.Logf(slogger.ERROR, "%v", err)
			if err = notify.NotifyAdmins(subject, err.Error(), s); err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Error sending email: %v", err)
			}
		}
		// close the channel to indicate the process is finished
		close(doneChan)
	}(c)
	return c
}

// startRunners starts a goroutine for each runner exposed via Runners. It
// returns a channel on which all runners listen on, for when to terminate.
func startRunners(wg *sync.WaitGroup, s *evergreen.Settings) chan struct{} {
	c := make(chan struct{}, 1)
	for _, r := range Runners {
		wg.Add(1)

		// start each runner in its own goroutine
		go func(r ProcessRunner, s *evergreen.Settings, terminateChan chan struct{}) {
			defer wg.Done()
			evergreen.Logger.Logf(slogger.INFO, "Starting %v", r.Name())
			// start the runner immediately
			processChan := runProcess(r, s)

			loop := true
			for loop {
				select {
				case <-terminateChan:
					loop = false
				case <-processChan:
					time.Sleep(time.Duration(runInterval) * time.Second)
					processChan = runProcess(r, s)
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

// runProcessByName runs a single process given its name and evergreen Settings.
// Returns an error if the process does not exist.
func runProcessByName(name string, settings *evergreen.Settings) error {
	for _, r := range Runners {
		if r.Name() == name {
			evergreen.Logger.Logf(slogger.INFO, "Running standalone %v process", name)
			if err := r.Run(settings); err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Error: %v", err)
			}
			return nil
		}
	}
	return fmt.Errorf("process name '%v' does not exist", name)
}
