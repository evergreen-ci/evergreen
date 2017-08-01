// Main package for the Evergreen runner.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/hostinit"
	"github.com/evergreen-ci/evergreen/notify"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	. "github.com/evergreen-ci/evergreen/runner"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

func init() {
	usageTemplate := template.Must(template.New("usage").Parse(
		`{{.Program}} runs the Evergreen processes at regular intervals.

Usage:
  {{.Program}} [flags] [process name]

Supported flags are:
{{.FlagDefaults}}
Supported proccesses are:{{range .Runners}}
  {{.Name}}: {{.Description}}{{end}}

Pass a single process name to run that process once,
or leave [process name] blank to run all processes
at regular intervals.
`))

	flag.Usage = func() {
		// capture the default flag output for use in the template
		flagDefaults := &bytes.Buffer{}
		flag.CommandLine.SetOutput(flagDefaults)
		flag.CommandLine.PrintDefaults()

		// execute the usage template
		usageTemplate.Execute(os.Stderr, struct {
			Program      string
			FlagDefaults string
			Runners      []ProcessRunner
		}{os.Args[0], flagDefaults.String(), Runners})
	}
}

const (
	schedulerInterval  = 20 * time.Second
	hostinitInterval   = 20 * time.Second
	defaultRunInterval = 60 * time.Second
)

func main() {
	settings := evergreen.GetSettingsOrExit()
	sender, err := settings.GetSender(settings.Runner.LogFile)
	grip.CatchEmergencyFatal(err)
	defer sender.Close()
	grip.CatchEmergencyFatal(grip.SetSender(sender))
	evergreen.SetLegacyLogger()
	grip.SetName("evg-runner")
	grip.SetDefaultLevel(level.Info)
	grip.SetThreshold(level.Debug)

	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	home := evergreen.FindEvergreenHome()
	if home == "" {
		grip.EmergencyFatal("EVGHOME environment variable must be set to execute runner")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go evergreen.SystemInfoCollector(ctx)

	defer util.RecoverAndLogStackTrace()

	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(settings))

	// just run one process if an argument was passed in
	if flag.Arg(0) != "" {
		grip.CatchEmergencyFatal(runProcessByName(flag.Arg(0), settings))
	}

	// start and schedule runners
	wg := &sync.WaitGroup{}
	ch := startRunners(wg, settings)
	go listenForSIGTERM(ch)

	// wait for all the processes to exit
	wg.Wait()
	grip.Infof("Cleanly terminated all %d processes", len(Runners))
}

// startRunners starts a goroutine for each runner exposed via Runners. It
// returns a channel on which all runners listen on, for when to terminate.
func startRunners(wg *sync.WaitGroup, s *evergreen.Settings) chan bool {
	c := make(chan bool)

	duration := defaultRunInterval
	if s.Runner.IntervalSeconds > 0 {
		duration = time.Duration(s.Runner.IntervalSeconds) * time.Second
	}

	grip.Noticef("runner periodicity set to %s (default: %s, scheduler: %s)",
		time.Duration(s.Runner.IntervalSeconds)*time.Second,
		defaultRunInterval, schedulerInterval)

	for _, r := range Runners {
		wg.Add(1)

		if r.Name() == scheduler.RunnerName {
			go runnerBackgroundWorker(r, s, c, schedulerInterval, wg)
		} else if r.Name() == hostinit.RunnerName {
			go runnerBackgroundWorker(r, s, c, hostinitInterval, wg)
		} else {
			go runnerBackgroundWorker(r, s, c, duration, wg)
		}
	}
	return c
}

func runnerBackgroundWorker(r ProcessRunner, s *evergreen.Settings, terminateChan chan bool, dur time.Duration, wg *sync.WaitGroup) {
	timer := time.NewTimer(0)
	defer wg.Done()

	grip.Infoln("Starting runner process:", r.Name())

	for {
		select {
		case <-terminateChan:
			grip.Infoln("Cleanly terminated runner process:", r.Name())
			return
		case <-timer.C:
			if err := r.Run(s); err != nil {
				subject := fmt.Sprintf("%s failure", r.Name())
				grip.Error(err)
				if err = notify.NotifyAdmins(subject, err.Error(), s); err != nil {
					grip.Errorf("sending email: %+v", err)
				}
			}

			grip.Debugln("restarting runner loop for:", r.Name())
			timer.Reset(dur)
		}
	}
}

// listenForSIGTERM listens for the SIGTERM signal and closes the
// channel on which each runner is listening as soon as the signal
// is received.
func listenForSIGTERM(ch chan bool) {
	sigChan := make(chan os.Signal)
	// notify us when SIGTERM is received
	signal.Notify(sigChan, syscall.SIGTERM)
	<-sigChan
	grip.Infof("Terminating %d processes", len(Runners))
	close(ch)
}

// runProcessByName runs a single process given its name and evergreen Settings.
// Returns an error if the process does not exist.
func runProcessByName(name string, settings *evergreen.Settings) error {
	for _, r := range Runners {
		if r.Name() == name {
			grip.Infof("Running standalone %s process", name)
			if err := r.Run(settings); err != nil {
				grip.Error(err)
			}
			return nil
		}
	}
	return errors.Errorf("process '%s' does not exist", name)
}
