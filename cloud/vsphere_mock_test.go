// +build go1.7

package cloud

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
	"github.com/vmware/govmomi/vim25/types"
)

type vsphereClientMock struct {
	// API call options
	failInit       bool
	failIP         bool
	failPowerState bool
	failCreate     bool
	failDelete     bool

	// Other options
	isActive bool
}

func (c *vsphereClientMock) Init(context.Context, *authOptions) error {
	if c.failInit {
		return errors.New("failed to initialize instance")
	}

	return nil
}

func (c *vsphereClientMock) GetIP(context.Context, *host.Host) (string, error) {
	if c.failIP {
		return "", errors.New("failed to get IP")
	}

	return "0.0.0.0", nil
}

func (c *vsphereClientMock) GetPowerState(context.Context, *host.Host) (types.VirtualMachinePowerState, error) {
	if c.failPowerState {
		err := errors.New("failed to read power state")
		return types.VirtualMachinePowerState(""), err
	}

	if !c.isActive {
		return types.VirtualMachinePowerStatePoweredOff, nil
	}

	return types.VirtualMachinePowerStatePoweredOn, nil
}

func (c *vsphereClientMock) CreateInstance(ctx context.Context, h *host.Host, _ *vsphereSettings) (string, error) {
	if c.failCreate {
		return "", errors.New("failed to create instance")
	}

	return h.Id, nil
}

func (c *vsphereClientMock) DeleteInstance(context.Context, *host.Host) error {
	if c.failDelete {
		return errors.New("failed to delete instance")
	}

	return nil
}
