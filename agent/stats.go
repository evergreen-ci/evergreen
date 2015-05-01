package agent

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/command"
	"time"
)

type StatsCollector struct {
	logger   *slogger.Logger
	Cmds     []string
	Interval time.Duration
	// A channel which, when closed, tells the stats collector ticker to stop.
	stop chan bool
}

func NewSimpleStatsCollector(logger *slogger.Logger, interval time.Duration,
	exp *command.Expansions, rawCmds ...string) *StatsCollector {
	expandedCmds := []string{}
	for _, cmd := range rawCmds {
		expanded, err := exp.ExpandString(cmd)
		if err != nil {
			logger.Logf(slogger.ERROR, "Couldn't expand stats command, "+
				"skipping: %v", err)
			continue
		}
		expandedCmds = append(expandedCmds, expanded)
	}
	return &StatsCollector{
		logger:   logger,
		Cmds:     expandedCmds,
		Interval: interval,
	}
}

func (self *StatsCollector) LogStats() {
	if self.Interval < 0 {
		panic(fmt.Sprintf("Illegal interval: %v", self.Interval))
	}
	if self.stop != nil {
		panic("StatsCollector goroutine already running!")
	}
	if self.Interval == 0 {
		self.Interval = 60 * time.Second
	}
	sysloggerInfoWriter := evergreen.NewInfoLoggingWriter(self.logger)
	sysloggerErrWriter := evergreen.NewErrorLoggingWriter(self.logger)
	self.stop = make(chan bool)
	go func() {
		ticker := time.NewTicker(self.Interval)
		for {
			select {
			case <-ticker.C:
				for _, cmd := range self.Cmds {
					self.logger.Logf(slogger.INFO, "Running %v", cmd)
					command := &command.LocalCommand{
						CmdString: cmd,
						Stdout:    sysloggerInfoWriter,
						Stderr:    sysloggerErrWriter,
					}
					err := command.Run()
					if err != nil {
						self.logger.Logf(slogger.ERROR, "system stats command "+
							"exited with err: %v", err)
					}
				}
			case <-self.stop:
				self.logger.Logf(slogger.INFO, "StatsCollector ticker stopping.")
				ticker.Stop()
				self.stop = nil
				return
			}
		}
	}()
}
