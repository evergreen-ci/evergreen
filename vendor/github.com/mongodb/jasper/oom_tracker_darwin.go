// +build darwin

package jasper

import (
	"context"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkg/errors"
)

func (o *oomTrackerImpl) Clear(ctx context.Context) error {
	sudo, err := isSudo(ctx)
	if err != nil {
		return errors.Wrap(err, "error checking sudo")
	}

	if sudo {
		return errors.Wrap(exec.CommandContext(ctx, "sudo", "log", "erase", "--all").Run(), "error clearing log")
	}

	return errors.Wrap(exec.CommandContext(ctx, "log", "erase", "--all").Run(), "error clearing log")
}

func (o *oomTrackerImpl) Check(ctx context.Context) error {
	analyzer := logAnalyzer{
		cmdArgs:        []string{"log", "show"},
		lineHasOOMKill: logContainsOOMKill,
		extractPID:     getPIDFromLog,
	}
	wasOOMKilled, pids, err := analyzer.analyzeKernelLog(ctx)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.PIDs = pids
	return nil
}

func getPIDFromLog(line string) (int, bool) {
	r := regexp.MustCompile(`pid (\d+)`)
	matches := r.FindStringSubmatch(line)
	if len(matches) != 2 {
		return 0, false
	}
	pid, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0, false
	}
	return pid, true
}

func logContainsOOMKill(line string) bool {
	return strings.Contains(line, "low swap")
}
