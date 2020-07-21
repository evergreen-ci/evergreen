// +build linux

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
		return errors.Wrap(exec.CommandContext(ctx, "sudo", "dmesg", "-c").Run(), "error clearing dmesg")
	}

	return errors.Wrap(exec.CommandContext(ctx, "dmesg", "-c").Run(), "error clearing dmesg")
}

func (o *oomTrackerImpl) Check(ctx context.Context) error {
	analyzer := logAnalyzer{
		cmdArgs:        []string{"dmesg"},
		lineHasOOMKill: dmesgContainsOOMKill,
		extractPID:     getPIDFromDmesg,
	}
	wasOOMKilled, pids, err := analyzer.analyzeKernelLog(ctx)
	if err != nil {
		return errors.Wrap(err, "error searching log")
	}
	o.WasOOMKilled = wasOOMKilled
	o.PIDs = pids
	return nil
}

func dmesgContainsOOMKill(line string) bool {
	return strings.Contains(line, "Out of memory") ||
		strings.Contains(line, "Killed process") || strings.Contains(line, "OOM killer") ||
		strings.Contains(line, "OOM-killer")
}

func getPIDFromDmesg(line string) (int, bool) {
	r := regexp.MustCompile(`Killed process (\d+)`)
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
