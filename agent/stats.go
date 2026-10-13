package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// StatsCollector samples machine statistics and logs them
// back to the API server at regular intervals.
type StatsCollector struct {
	logger client.LoggerProducer
	jasper jasper.Manager
	// Cmds are agent-fixed host diagnostics, never task-controlled.
	Cmds []string
	// PSCmd is the task-configurable ps command. It runs inside the task's
	// isolation container when one exists because its value is
	// author-controlled.
	PSCmd string
	// Container fields locate the task's isolation container, used only to
	// run PSCmd.
	ContainerID    string
	WorkDir        string
	EnvFileHostDir string
	ExecUser       string
	// indicates the sampling frequency
	Interval time.Duration
}

// NewSimpleStatsCollector creates a StatsCollector that runs the given commands
// at the given interval and sends the results to the given logger.
func NewSimpleStatsCollector(logger client.LoggerProducer, jpm jasper.Manager, interval time.Duration, cmds ...string) *StatsCollector {
	return &StatsCollector{
		logger:   logger,
		Cmds:     cmds,
		Interval: interval,
		jasper:   jpm,
	}
}

// setPSCommand records the ps command and the isolation container info
// needed to run it.
func (sc *StatsCollector) setPSCommand(psCmd string, conf *internal.TaskConfig) {
	sc.PSCmd = psCmd
	if conf == nil {
		return
	}
	sc.ContainerID = conf.ContainerID
	sc.WorkDir = conf.WorkDir
	sc.EnvFileHostDir = conf.EnvFileHostDir
	if conf.Distro != nil {
		sc.ExecUser = conf.Distro.ExecUser
	}
}

func (sc *StatsCollector) expandCommands(ctx context.Context, exp util.Expansions) {
	expandedCmds := []string{}
	for _, cmd := range sc.Cmds {
		expanded, err := exp.ExpandString(cmd)
		if err != nil {
			sc.logger.System().Warning(ctx, errors.Wrapf(err, "expanding stats command '%s'", cmd))
			continue
		}
		if strings.TrimSpace(expanded) == "" {
			continue
		}
		expandedCmds = append(expandedCmds, expanded)
	}
	sc.Cmds = expandedCmds
}

func (sc *StatsCollector) logStats(ctx context.Context, exp util.Expansions) {
	if sc.Interval < 0 {
		panic(fmt.Sprintf("Illegal stats collection interval: %s", sc.Interval))
	}
	if sc.Interval == 0 {
		sc.Interval = 60 * time.Second
	}
	sc.expandCommands(ctx, exp)

	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		defer recovery.LogStackTraceAndContinue("encountered issue in stats collector")

		sc.logger.System().Infof(ctx, "Starting stats collector with %d commands at interval %s: %s", len(sc.Cmds), sc.Interval, strings.Join(sc.Cmds, ", "))

		iters := 0
		startedAt := time.Now()
		for {
			iters++
			select {
			case <-ctx.Done():
				sc.logger.System().Info(ctx, "StatsCollector ticker stopping.")
				return
			case <-timer.C:
				runStartedAt := time.Now()
				sc.runCollection(ctx, iters, runStartedAt, startedAt)
				timer.Reset(sc.Interval)
			}
		}
	}()
}

// runCollection runs the host diagnostics and the ps command once, then logs
// the outcome.
func (sc *StatsCollector) runCollection(ctx context.Context, iters int, runStartedAt, startedAt time.Time) {
	err := sc.runCommands(ctx, sc.Cmds)
	sc.logCollectionResult(ctx, err, "host stats collector", iters, runStartedAt, startedAt)

	if sc.PSCmd != "" {
		psErr := sc.runPSCommand(ctx)
		sc.logCollectionResult(ctx, psErr, "ps stats collector", iters, runStartedAt, startedAt)
	}
}

func (sc *StatsCollector) runCommands(ctx context.Context, cmds []string) error {
	if len(cmds) == 0 {
		return nil
	}
	return sc.jasper.CreateCommand(ctx).Append(cmds...).
		ContinueOnError(true).
		SetOutputSender(level.Info, sc.logger.System().GetSender()).
		SetErrorSender(level.Error, sc.logger.System().GetSender()).
		Run(ctx)
}

// runPSCommand runs the ps command inside the task's isolation container
// when one exists. If the wrapping fails, the command is not run.
func (sc *StatsCollector) runPSCommand(ctx context.Context) error {
	cmd := sc.jasper.CreateCommand(ctx).
		ContinueOnError(true).
		SetOutputSender(level.Info, sc.logger.System().GetSender()).
		SetErrorSender(level.Error, sc.logger.System().GetSender()).
		ProcConstructor(func(pctx context.Context, opts *options.Create) (jasper.Process, error) {
			if err := sc.wrapPSOptions(pctx, opts); err != nil {
				return nil, err
			}
			return sc.jasper.CreateProcess(pctx, opts)
		}).
		Append(sc.PSCmd)
	if sc.ContainerID != "" && sc.ExecUser != "" {
		cmd.SudoAs(sc.ExecUser)
	}
	return cmd.Run(ctx)
}

// wrapPSOptions rewrites the process options to run inside the task's
// isolation container. It is a no-op without a container.
func (sc *StatsCollector) wrapPSOptions(ctx context.Context, opts *options.Create) error {
	if sc.ContainerID == "" {
		return nil
	}
	return agentutil.WrapWithContainer(ctx, opts, sc.ContainerID, sc.WorkDir, sc.EnvFileHostDir)
}

// logCollectionResult logs the outcome of one collection iteration. The error
// branch must only log non-nil errors, since a wrapped nil error is dropped
// by grip before rendering.
func (sc *StatsCollector) logCollectionResult(ctx context.Context, err error, name string, iters int, runStartedAt, startedAt time.Time) {
	fields := message.Fields{
		"iterations":        iters,
		"iter_runtime_secs": time.Since(runStartedAt).Seconds(),
		"runtime_secs":      time.Since(startedAt).Seconds(),
		"interval":          sc.Interval,
	}
	if err != nil {
		fields["message"] = fmt.Sprintf("error running %s", name)
		sc.logger.System().Error(ctx, message.WrapError(err, fields))
		return
	}
	fields["message"] = fmt.Sprintf("ran %s", name)
	sc.logger.System().Debug(ctx, fields)
}
