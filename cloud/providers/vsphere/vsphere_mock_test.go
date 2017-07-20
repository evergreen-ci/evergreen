// +build go1.7

package vsphere

import (
	"github.com/pkg/errors"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/vmware/govmomi/vim25/types"
)

type clientMock struct {
	// API call options
	failInit       bool
	failIP         bool
	failPowerState bool
	failCreate     bool
	failDelete     bool

	// Other options
	isActive bool
}

func (c *clientMock) Init(_ *authOptions) error {
	if c.failInit {
		return errors.New("failed to initialize instance")
	}

	return nil
}

func (c *clientMock) GetIP(_ *host.Host) (string, error) {
	if c.failIP {
		return "", errors.New("failed to get IP")
	}

	return "0.0.0.0", nil
}

func (c *clientMock) GetPowerState(_ *host.Host) (types.VirtualMachinePowerState, error) {
	if c.failPowerState {
		err := errors.New("failed to read power state")
		return types.VirtualMachinePowerState(""), err
	}

	if !c.isActive {
		return types.VirtualMachinePowerStatePoweredOff, nil
	}

	return types.VirtualMachinePowerStatePoweredOn, nil
}

func (c *clientMock) CreateInstance(h *host.Host, _ *ProviderSettings) (string, error) {
	if c.failCreate {
		return "", errors.New("failed to create instance")
	}

	return h.Id, nil
}

func (c *clientMock) DeleteInstance(_ *host.Host) error {
	if c.failDelete {
		return errors.New("failed to delete instance")
	}

	return nil
}
