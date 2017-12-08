// +build !go1.7

package vsphere

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

const (
	ProviderName = "vsphere"
)

// nolint
type vsphereManager struct{}

type vsphereSettings struct{}

func (opts *vsphereSettings) Validate() error {
	return nil
}

func (m *vsphereManager) GetSettings() ProviderSettings {
	return &vsphereSettings{}
}

func (m *vsphereManager) Configure(_ *evergreen.Settings) error {
	return nil
}

func (m *vsphereManager) SpawnHost(*host.Host) (*host.Host, error) {
	return &host.Host{}, nil
}

func (m *vsphereManager) CanSpawn() (bool, error) {
	return true, nil
}

func (m *vsphereManager) GetInstanceStatus(_ *host.Host) (CloudStatus, error) {
	return StatusRunning, nil
}

func (*vsphereManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

func (m *vsphereManager) TerminateInstance(_ *host.Host) error {
	return nil
}

func (m *vsphereManager) IsUp(_ *host.Host) (bool, error) {
	return true, nil
}

func (m *vsphereManager) OnUp(_ *host.Host) error {
	return nil
}

func (m *vsphereManager) IsSSHReachable(_ *host.Host, _ string) (bool, error) {
	return true, nil
}

func (m *vsphereManager) GetDNSName(_ *host.Host) (string, error) {
	return "0.0.0.0", nil
}

func (m *vsphereManager) GetSSHOptions(_ *host.Host, _ string) ([]string, error) {
	return []string{}, nil
}

func (m *vsphereManager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}
