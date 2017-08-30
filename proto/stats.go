package proto

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/subprocess"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"golang.org/x/net/context"
)

// StatsCollector samples machine statistics and logs them
// back to the API server at regular intervals.
type StatsCollector struct {
	logger client.LoggerProducer
	Cmds   []string
	// indicates the sampling frequency
	Interval time.Duration
}

// NewSimpleStatsCollector creates a StatsCollector that runs the given commands
// at the given interval and sends the results to the given logger.
func NewSimpleStatsCollector(logger client.LoggerProducer, interval time.Duration, cmds ...string) *StatsCollector {
	return &StatsCollector{
		logger:   logger,
		Cmds:     cmds,
		Interval: interval,
	}
}

func (sc *StatsCollector) expandCommands(exp *util.Expansions) {
	expandedCmds := []string{}
	for _, cmd := range sc.Cmds {
		expanded, err := exp.ExpandString(cmd)
		if err != nil {
			sc.logger.System().Warningf("Couldn't expand '%s': %v", cmd, err)
			continue
		}
		expandedCmds = append(expandedCmds, expanded)
	}
	sc.Cmds = expandedCmds
}

func (sc *StatsCollector) logStats(ctx context.Context, exp *util.Expansions) {
	if sc.Interval < 0 {
		panic(fmt.Sprintf("Illegal interval: %v", sc.Interval))
	}
	if sc.Interval == 0 {
		sc.Interval = 60 * time.Second
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	sc.expandCommands(exp)

	go func() {
		var cancel context.CancelFunc
		ctx, cancel = context.WithCancel(ctx)
		defer cancel()
		for {
			select {
			case <-ctx.Done():
				grip.Info("StatsCollector ticker stopping.")
				return
			case <-timer.C:
				for _, cmd := range sc.Cmds {
					sc.logger.System().Infof("Running %v", cmd)
					command := &subprocess.LocalCommand{
						CmdString: cmd,
						Stdout:    sc.logger.SystemWriter(level.Info),
						Stderr:    sc.logger.SystemWriter(level.Error),
					}
					if err := command.Run(ctx); err != nil {
						sc.logger.System().Errorf("error running '%v': %v", cmd, err)
					}
				}
				timer.Reset(sc.Interval)
			}
		}
	}()
}
