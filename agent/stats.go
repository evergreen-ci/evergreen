package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// StatsCollector samples machine statistics and logs them
// back to the API server at regular intervals.
type StatsCollector struct {
	logger client.LoggerProducer
	jasper jasper.Manager
	Cmds   []string
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

func (sc *StatsCollector) expandCommands(exp util.Expansions) {
	expandedCmds := []string{}
	for _, cmd := range sc.Cmds {
		expanded, err := exp.ExpandString(cmd)
		if err != nil {
			sc.logger.System().Warning(errors.Wrapf(err, "expanding stats command '%s'", cmd))
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
	sc.expandCommands(exp)

	go func() {
		timer := time.NewTimer(0)
		defer timer.Stop()
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		defer recovery.LogStackTraceAndContinue("encountered issue in stats collector")

		sc.logger.System().Infof("Starting stats collector with %d commands at interval %s: %s", len(sc.Cmds), sc.Interval, strings.Join(sc.Cmds, ", "))

		iters := 0
		startedAt := time.Now()
		for {
			iters++
			select {
			case <-ctx.Done():
				sc.logger.System().Info("StatsCollector ticker stopping.")
				return
			case <-timer.C:
				runStartedAt := time.Now()
				err := sc.jasper.CreateCommand(ctx).Append(sc.Cmds...).
					ContinueOnError(true).
					SetOutputSender(level.Info, sc.logger.System().GetSender()).
					SetErrorSender(level.Error, sc.logger.System().GetSender()).
					Run(ctx)

				sc.logger.System().Error(message.WrapError(err, message.Fields{
					"message":           "error running stats collector",
					"iterations":        iters,
					"iter_runtime_secs": time.Since(runStartedAt).Seconds(),
					"runtime_secs":      time.Since(startedAt).Seconds(),
					"interval":          sc.Interval,
				}))
				timer.Reset(sc.Interval)
			}
		}
	}()
}
