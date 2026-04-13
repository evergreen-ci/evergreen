//go:build !linux

package agent

import "context"

// activeMountsUnder returns nil on non-Linux platforms.
func activeMountsUnder(_ context.Context, _ string) []string {
	return nil
}
