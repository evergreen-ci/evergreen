// +build go1.7

package cloud

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/object"
	"github.com/vmware/govmomi/vim25/types"
)

// Powered on means the instance is powered on and running.
// Powered off means the instance is completely powered off.
// Suspended means the instance is in a hibernated/sleep state.
func vsphereToEvgStatus(state types.VirtualMachinePowerState) CloudStatus {
	switch state {
	case types.VirtualMachinePowerStatePoweredOn:
		return StatusRunning
	case types.VirtualMachinePowerStatePoweredOff:
		return StatusStopped
	case types.VirtualMachinePowerStateSuspended:
		return StatusStopped
	default:
		return StatusUnknown
	}
}

// getInstance gets a reference to this instance from the vCenter.
// If this method errors, it is possible the instance does not exist.
func (c *clientImpl) getInstance(ctx context.Context, name string) (*object.VirtualMachine, error) {
	vm, err := c.Finder.VirtualMachine(ctx, name)
	if err != nil {
		return nil, errors.Wrapf(err, "could not find vm for host %s", name)
	}

	return vm, err
}

// relocateSpec creates a spec for where the new machine will be located.
func (c *clientImpl) relocateSpec(s *ProviderSettings) (types.VirtualMachineRelocateSpec, error) {
	ctx := context.TODO()
	var spec types.VirtualMachineRelocateSpec
	var morRP types.ManagedObjectReference
	var morDS types.ManagedObjectReference

	// resource pool is required
	rp, err := c.Finder.ResourcePool(ctx, s.ResourcePool)
	if err != nil {
		err = errors.Wrapf(err, "error finding pool %s", s.ResourcePool)
		grip.Error(err)
		return spec, err
	}
	morRP = rp.Common.Reference()
	spec.Pool = &morRP

	if s.Datastore != "" {
		ds, err := c.Finder.Datastore(ctx, s.Datastore)
		if err != nil {
			err = errors.Wrapf(err, "error finding datastore %s", s.Datastore)
			grip.Error(err)
			return spec, err
		}
		morDS = ds.Common.Reference()
		spec.Datastore = &morDS
	}

	grip.Info(message.Fields{
		"message":       "created spec to relocate clone",
		"resource_pool": morRP,
		"datastore":     morDS,
	})

	return spec, nil
}

// configSpec creates a spec for hardware configuration of the new machine.
func (c *clientImpl) configSpec(s *ProviderSettings) types.VirtualMachineConfigSpec {
	spec := types.VirtualMachineConfigSpec{
		NumCPUs:  s.NumCPUs,
		MemoryMB: s.MemoryMB,
	}

	// the new vm will default to the clone's original values if either option is 0
	grip.Info(message.Fields{
		"message":   "created spec to reconfigure clone",
		"num_cpus":  spec.NumCPUs,
		"memory_mb": spec.MemoryMB,
	})

	return spec
}

// cloneSpec creates a spec for a new virtual machine, specifying
// where it will start up and its hardware configurations.
func (c *clientImpl) cloneSpec(s *ProviderSettings) (types.VirtualMachineCloneSpec, error) {
	var spec types.VirtualMachineCloneSpec

	cs := c.configSpec(s)
	rs, err := c.relocateSpec(s)
	if err != nil {
		return spec, errors.Wrap(err, "error making relocate spec")
	}

	spec = types.VirtualMachineCloneSpec{
		Config:   &cs,
		Location: rs,
		Template: false,
		PowerOn:  true,
	}

	return spec, nil
}
