// +build go1.7

package vsphere

import (
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/vmware/govmomi/vim25/types"
)

// Powered on means the instance is powered on and running.
// Powered off means the instance is completely powered off.
// Suspended means the instance is in a hibernated/sleep state.
func toEvgStatus(state types.VirtualMachinePowerState) cloud.CloudStatus {
	switch state {
	case types.VirtualMachinePowerStatePoweredOn:
		return cloud.StatusRunning
	case types.VirtualMachinePowerStatePoweredOff:
		return cloud.StatusStopped
	case types.VirtualMachinePowerStateSuspended:
		return cloud.StatusStopped
	default:
		return cloud.StatusUnknown
	}
}
