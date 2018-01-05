package operations

import (
	"context"
	"fmt"
	"math/rand"
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
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/urfave/cli"
)

func setupRunner() cli.BeforeFunc {
	return func(c *cli.Context) error {
		grip.SetName("evergreen.runner")

		if home := evergreen.FindEvergreenHome(); home == "" {
			grip.EmergencyFatal("EVGHOME environment variable must be set to execute runner")
		}

		return nil
	}
}

func handcrankRunner() cli.Command {
	return cli.Command{
		Name:    "handcrank",
		Usage:   "run a single background process by name",
		Aliases: []string{"run-single", "single"},
		Flags: serviceConfigFlags(cli.StringFlag{
			Name: joinFlagNames("runner", "r", "n", "name", "single"),
		}),
		Before: mergeBeforeFuncs(setupRunner(), requireFileExists(confFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)
			name := c.String("runner")
			if name == "" {
				return errors.New("must specify a runner")
			}

			ctx, cancel := context.WithCancel(context.Background()) // nolint
			env := evergreen.GetEnvironment()
			defer recovery.LogStackTraceAndExit("evergreen runner")
			defer cancel()

			err := env.Configure(ctx, confPath)
			if err != nil {
				return errors.Wrap(err, "problem configuring application environment")
			}

			grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name(), "mode": "single"})

			settings := env.Settings()

			return runProcessByName(ctx, name, settings)
		},
	}
}

func startRunnerService() cli.Command {
	return cli.Command{
		Name:   "runner",
		Usage:  "run evergreen background worker",
		Flags:  serviceConfigFlags(),
		Before: mergeBeforeFuncs(setupRunner(), requireFileExists(confFlagName)),
		Action: func(c *cli.Context) error {
			confPath := c.String(confFlagName)

			ctx, cancel := context.WithCancel(context.Background())
			env := evergreen.GetEnvironment()
			grip.CatchEmergencyFatal(errors.Wrap(env.Configure(ctx, confPath), "problem configuring application environment"))

			settings := env.Settings()
			sender, err := settings.GetSender(env)
			grip.CatchEmergencyFatal(err)
			grip.CatchEmergencyFatal(grip.SetSender(sender))

			defer sender.Close()
			defer recovery.LogStackTraceAndExit("evergreen runner")
			defer cancel()

			grip.Notice(message.Fields{"build": evergreen.BuildRevision, "process": grip.Name()})

			startCollectorJobs(ctx, env)
			go func() {
				defer recovery.LogStackTraceAndContinue("pprof server")
				if settings.PprofPort != "" {
					pprofServer := service.GetServer(settings.PprofPort, service.GetHandlerPprof(settings))
					grip.Alert(pprofServer.ListenAndServe())
				}
			}()

			// start and schedule runners
			//
			go listenForSIGTERM(cancel)
			waiter := make(chan struct{})
			startRunners(ctx, settings, waiter)
			grip.Notice("waiting for runner processes to terminate")
			<-waiter

			return errors.WithStack(err)
		},
	}
}

////////////////////////////////////////////////////////////////////////
//
// Running and executing the offline operations processing.

func startCollectorJobs(ctx context.Context, env evergreen.Environment) {
	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), time.Minute, time.Now(), true, func(queue amboy.Queue) error {
		catcher := grip.NewBasicCatcher()
		ts := time.Now().Unix()

		catcher.Add(queue.Put(units.NewAmboyStatsCollector(env, fmt.Sprintf("amboy-stats-%d", ts))))
		catcher.Add(queue.Put(units.NewHostStatsCollector(fmt.Sprintf("host-stats-%d", ts))))
		catcher.Add(queue.Put(units.NewTaskStatsCollector(fmt.Sprintf("task-stats-%d", ts))))
		catcher.Add(queue.Put(units.NewLatencyStatsCollector(fmt.Sprintf("latency-stats-%d", ts), time.Minute)))

		return catcher.Resolve()
	})

	amboy.IntervalQueueOperation(ctx, env.LocalQueue(), 15*time.Second, time.Now(), true, func(queue amboy.Queue) error {
		return queue.Put(units.NewSysInfoStatsCollector(fmt.Sprintf("sys-info-stats-%d", time.Now().Unix())))
	})
}

type processRunner interface {
	// Name returns the id of the process runner.
	Name() string

	// Run executes the process runner with the supplied configuration.
	Run(context.Context, *evergreen.Settings) error
}

var backgroundRunners = []processRunner{
	&alerts.QueueProcessor{},
	&notify.Runner{},
	&repotracker.Runner{},

	&scheduler.Runner{},
	&hostinit.Runner{},
	&taskrunner.Runner{},

	&monitor.Runner{},
}

// startRunners starts a goroutine for each runner exposed via Runners. It
// returns a channel on which all runners listen on, for when to terminate.
func startRunners(ctx context.Context, s *evergreen.Settings, waiter chan struct{}) {
	const (
		frequentRunInterval   = 20 * time.Second
		defaultRunInterval    = 60 * time.Second
		infrequentRunInterval = 120 * time.Second
	)

	wg := &sync.WaitGroup{}

	frequentRunners := []string{
		scheduler.RunnerName,
		hostinit.RunnerName,
		taskrunner.RunnerName,
	}

	infrequentRunners := []string{
		alerts.RunnerName,
		notify.RunnerName,
		repotracker.RunnerName,
	}

	grip.Notice(message.Fields{
		"default_interval": defaultRunInterval,
		"default_span":     defaultRunInterval.String(),
		"frequent": message.Fields{
			"interval": frequentRunInterval,
			"span":     frequentRunInterval.String(),
			"runners":  frequentRunners,
		},
		"infrequent": message.Fields{
			"interval": infrequentRunInterval,
			"span":     infrequentRunInterval.String(),
			"runners":  infrequentRunners,
		},
	})

	for _, r := range backgroundRunners {
		wg.Add(1)
		if util.StringSliceContains(frequentRunners, r.Name()) {
			go runnerBackgroundWorker(ctx, r, s, frequentRunInterval, wg)
		} else if util.StringSliceContains(infrequentRunners, r.Name()) {
			go runnerBackgroundWorker(ctx, r, s, infrequentRunInterval, wg)
		} else {
			go runnerBackgroundWorker(ctx, r, s, defaultRunInterval, wg)
		}
	}

	wg.Wait()
	grip.Infof("Cleanly terminated all %d processes", len(backgroundRunners))
	close(waiter)
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
	timer := time.NewTimer(time.Duration(rand.Int63n(int64(dur))))
	defer wg.Done()
	defer timer.Stop()
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "worker process encountered error")
		if err != nil {
			wg.Add(1)
			go runnerBackgroundWorker(ctx, r, s, dur, wg)
		}
	}()

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
					grip.Error(errors.Wrap(err, "sending email"))
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
