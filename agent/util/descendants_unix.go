//go:build darwin || linux

package util

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

const maxDescendantDepth = 10

// GetDescendantPIDs returns all descendant PIDs of the given parent PIDs
// by recursively calling pgrep -P. Returns nil if pgrep is unavailable.
func GetDescendantPIDs(ctx context.Context, parentPIDs []int) []int {
	if len(parentPIDs) == 0 {
		return nil
	}

	seen := make(map[int]bool)
	var result []int
	queue := make([]int, len(parentPIDs))
	copy(queue, parentPIDs)
	for _, pid := range parentPIDs {
		seen[pid] = true
	}

	for depth := 0; depth < maxDescendantDepth && len(queue) > 0; depth++ {
		var nextQueue []int
		for _, pid := range queue {
			children := pgrepChildren(ctx, pid)
			for _, child := range children {
				if !seen[child] {
					seen[child] = true
					result = append(result, child)
					nextQueue = append(nextQueue, child)
				}
			}
		}
		queue = nextQueue
	}

	return result
}

// pgrepChildren returns the direct child PIDs of the given parent PID.
func pgrepChildren(ctx context.Context, parentPID int) []int {
	out, err := exec.CommandContext(ctx, "pgrep", "-P", strconv.Itoa(parentPID)).Output()
	if err != nil {
		return nil
	}

	var pids []int
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if pid, err := strconv.Atoi(line); err == nil {
			pids = append(pids, pid)
		}
	}
	return pids
}
