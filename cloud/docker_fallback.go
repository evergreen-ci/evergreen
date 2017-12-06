// +build !go1.7

package cloud

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

const (
	ProviderName = "docker"
)

type dockerManager struct{}

type ProviderSettings struct{}

func (opts *ProviderSettings) Validate() error {
	return nil
}

func (m *dockerManager) GetSettings() ProviderSettings {
	return &dockerProviderSettings{}
}

func (m *dockerManager) Configure(_ *evergreen.Settings) error {
	return nil
}

func (m *dockerManager) SpawnHost(*host.Host) (*host.Host, error) {
	return &host.Host{}, nil
}

func (m *dockerManager) CanSpawn() (bool, error) {
	return true, nil
}

func (m *dockerManager) GetInstanceStatus(_ *host.Host) (CloudStatus, error) {
	return StatusRunning, nil
}

func (*dockerManager) GetInstanceName(d *distro.Distro) string {
	return d.GenerateName()
}

func (m *dockerManager) TerminateInstance(_ *host.Host) error {
	return nil
}

func (m *dockerManager) IsUp(_ *host.Host) (bool, error) {
	return true, nil
}

func (m *dockerManager) OnUp(_ *host.Host) error {
	return nil
}

func (m *dockerManager) IsSSHReachable(_ *host.Host, _ string) (bool, error) {
	return true, nil
}

func (m *dockerManager) GetDNSName(_ *host.Host) (string, error) {
	return "0.0.0.0", nil
}

func (m *dockerManager) GetSSHOptions(_ *host.Host, _ string) ([]string, error) {
	return []string{}, nil
}

func (m *dockerManager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}
