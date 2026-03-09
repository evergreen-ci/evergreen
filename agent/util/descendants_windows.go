package util

import "context"

// GetDescendantPIDs is a no-op on Windows. Windows uses job objects for
// process management, so child enumeration via pgrep is not applicable.
func GetDescendantPIDs(ctx context.Context, parentPIDs []int) []int {
	return nil
}
