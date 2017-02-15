package agent

import (
	"fmt"
	"time"

	"github.com/tychoish/grip/slogger"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
)

// StatsCollector samples machine statistics and logs them
// back to the API server at regular intervals.
type StatsCollector struct {
	logger *slogger.Logger
	Cmds   []string
	// indicates the sampling frequency
	Interval time.Duration
	// when closed this stops stats collector ticker
	stop <-chan struct{}
}

// NewSimpleStatsCollector creates a StatsCollector that runs the given commands
// at the given interval and sends the results to the given logger.
func NewSimpleStatsCollector(logger *slogger.Logger, interval time.Duration,
	stop <-chan struct{}, cmds ...string) *StatsCollector {
	return &StatsCollector{
		logger:   logger,
		Cmds:     cmds,
		Interval: interval,
		stop:     stop,
	}
}

func (sc *StatsCollector) expandCommands(exp *command.Expansions) {
	expandedCmds := []string{}
	for _, cmd := range sc.Cmds {
		expanded, err := exp.ExpandString(cmd)
		if err != nil {
			sc.logger.Logf(slogger.WARN, "Couldn't expand '%v': %v", cmd, err)
			continue
		}
		expandedCmds = append(expandedCmds, expanded)
	}
	sc.Cmds = expandedCmds
}

func (sc *StatsCollector) LogStats(exp *command.Expansions) {
	sc.expandCommands(exp)

	if sc.Interval < 0 {
		panic(fmt.Sprintf("Illegal interval: %v", sc.Interval))
	}
	if sc.Interval == 0 {
		sc.Interval = 60 * time.Second
	}

	sysloggerInfoWriter := evergreen.NewInfoLoggingWriter(sc.logger)
	sysloggerErrWriter := evergreen.NewErrorLoggingWriter(sc.logger)

	go func() {
		for {
			select {
			case <-sc.stop:
				sc.logger.Logf(slogger.INFO, "StatsCollector ticker stopping.")
				return
			default:
				for _, cmd := range sc.Cmds {
					sc.logger.Logf(slogger.INFO, "Running %v", cmd)
					command := &command.LocalCommand{
						CmdString: cmd,
						Stdout:    sysloggerInfoWriter,
						Stderr:    sysloggerErrWriter,
					}
					if err := command.Run(); err != nil {
						sc.logger.Logf(slogger.ERROR, "error running '%v': %v", cmd, err)
					}
				}
				time.Sleep(sc.Interval)
			}
		}
	}()
}
