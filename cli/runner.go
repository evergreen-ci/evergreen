package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/hostinit"
	"github.com/evergreen-ci/evergreen/monitor"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/repotracker"
	"github.com/evergreen-ci/evergreen/scheduler"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

const (
	frequentRunInterval = 20 * time.Second
	defaultRunInterval  = 60 * time.Second
)

type ServiceRunnerCommand struct {
	ConfigPath string `long:"conf" default:"/etc/mci_settings.yml" description:"path to the service configuration file"`
	Single     string `long:"single" default:"" description:"specify "`
}

func (c *ServiceRunnerCommand) Execute(_ []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := evergreen.GetEnvironment()
	if err := env.Configure(ctx, c.ConfigPath); err != nil {
		return errors.Wrap(err, "problem configuring application environment")
	}

	settings := env.Settings()

	sender, err := settings.GetSender()
	grip.CatchEmergencyFatal(err)
	defer sender.Close()
	grip.CatchEmergencyFatal(grip.SetSender(sender))
	grip.SetName("evg-runner")
	grip.Warning(grip.SetDefaultLevel(level.Info))
	grip.Warning(grip.SetThreshold(level.Debug))

	defer recovery.LogStackTraceAndExit("evergreen runner")

	grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

	if home := evergreen.FindEvergreenHome(); home == "" {
		grip.EmergencyFatal("EVGHOME environment variable must be set to execute runner")
	}

	// just run a single runner if only one was passed in.
	if c.Single != "" {
		return runProcessByName(ctx, c.Single, settings)
	}

	startCollectorJobs(ctx, env)

	pprofHandler := service.GetHandlerPprof(settings)
	if settings.PprofPort != "" {
		go func() {
			defer recovery.LogStackTraceAndContinue("pprof threads")
			grip.Alert(service.RunGracefully(settings.PprofPort, requestTimeout, pprofHandler))
		}()
	}

	// start and schedule runners
	//
	go listenForSIGTERM(cancel)
	startRunners(ctx, settings)

	return errors.WithStack(err)
}

func startCollectorJobs(ctx context.Context, env evergreen.Environment) {
	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), time.Minute, time.Now(), true, func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := time.Now().Unix()

		catcher.Add(queue.Put(units.NewAmboyStatsCollector(env, fmt.Sprintf("amboy-stats-%d", ts))))
		catcher.Add(queue.Put(units.NewHostStatsCollector(fmt.Sprintf("host-stats-%d", ts))))
		catcher.Add(queue.Put(units.NewTaskStatsCollector(fmt.Sprintf("task-stats-%d", ts))))

		return catcher.Resolve()
	})

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), 15*time.Second, time.Now(), true, func(queue amboy.Queue) error {
		return queue.Put(units.NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%d", time.Now().Unix())))
	})
}

////////////////////////////////////////////////////////////////////////
//
// Running and executing the offline operations processing.

type processRunner interface {
	// Name returns the id of the process runner.
	Name() string

	// Run executes the process runner with the supplied configuration.
	Run(context.Context, *evergreen.Settings) error
}

var backgroundRunners = []processRunner{
	&hostinit.Runner{},
	&monitor.Runner{},
	&notify.Runner{},
	&repotracker.Runner{},
	&taskrunner.Runner{},
	&alerts.QueueProcessor{},
	&scheduler.Runner{},
}

// startRunners starts a goroutine for each runner exposed via Runners. It
// returns a channel on which all runners listen on, for when to terminate.
func startRunners(ctx context.Context, s *evergreen.Settings) {
	wg := &sync.WaitGroup{}

	duration := defaultRunInterval
	if s.Runner.IntervalSeconds > 0 {
		duration = time.Duration(s.Runner.IntervalSeconds) * time.Second
	}

	frequentRunners := []string{
		scheduler.RunnerName,
		hostinit.RunnerName,
		taskrunner.RunnerName,
	}

	grip.Notice(message.Fields{
		"default_duration":  duration,
		"default_span":      duration.String(),
		"frequent_duration": frequentRunInterval,
		"frequent_span":     frequentRunInterval.String(),
		"frequent_runners":  frequentRunners,
	})

	for _, r := range backgroundRunners {
		wg.Add(1)

		if util.SliceContains(frequentRunners, r.Name()) {
			go runnerBackgroundWorker(ctx, r, s, frequentRunInterval, wg)
		} else {
			go runnerBackgroundWorker(ctx, r, s, duration, wg)
		}
	}

	wg.Wait()
	grip.Infof("Cleanly terminated all %d processes", len(backgroundRunners))
}

// listenForSIGTERM listens for the SIGTERM signal and closes the
// channel on which each runner is listening as soon as the signal
// is received.
func listenForSIGTERM(cancel context.CancelFunc) {
	sigChan := make(chan os.Signal, 5)
	// notify us when SIGTERM is received
	signal.Notify(sigChan, syscall.SIGTERM)
	<-sigChan
	grip.Infof("Terminating %d processes", len(backgroundRunners))
	cancel()
}

// runProcessByName runs a single process given its name and evergreen Settings.
// Returns an error if the process does not exist.
func runProcessByName(ctx context.Context, name string, settings *evergreen.Settings) error {
	for _, r := range backgroundRunners {
		if r.Name() == name {
			grip.Infof("Running standalone %s process", name)
			if err := r.Run(ctx, settings); err != nil {
				grip.Error(err)
			}
			return nil
		}
	}
	return errors.Errorf("process '%s' does not exist", name)
}

func runnerBackgroundWorker(ctx context.Context, r processRunner, s *evergreen.Settings, dur time.Duration, wg *sync.WaitGroup) {
	timer := time.NewTimer(0)
	defer wg.Done()
	defer timer.Stop()
	defer recovery.LogStackTraceAndContinue("background runner process for", r.Name())

	grip.Infoln("Starting runner process:", r.Name())

	for {
		select {
		case <-ctx.Done():
			grip.Infoln("Cleanly terminated runner process:", r.Name())
			return
		case <-timer.C:
			if err := r.Run(ctx, s); err != nil {
				grip.Info(message.Fields{
					"message":  "run complete, encountered error",
					"runner":   r.Name(),
					"interval": dur,
					"sleep":    dur.String(),
					"error":    err.Error(),
				})

				subject := fmt.Sprintf("%s failure", r.Name())
				if err = notify.NotifyAdmins(subject, err.Error(), s); err != nil {
					grip.Errorf("sending email: %+v", err)
				}

			} else {
				grip.Info(message.Fields{
					"message":  "run completed, successfully",
					"runner":   r.Name(),
					"interval": dur,
					"sleep":    dur.String(),
				})
			}

			timer.Reset(dur)
		}
	}
}
