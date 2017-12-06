// +build !go1.7

package gce

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

type Manager struct{}

type ProviderSettings struct{}

func (opts *ProviderSettings) Validate() error {
	return nil
}

func (m *Manager) GetSettings() cloud.ProviderSettings {
	return &ProviderSettings{}
}

func (m *Manager) Configure(_ *evergreen.Settings) error {
	return nil
}

func (m *Manager) SpawnHost(*host.Host) (*host.Host, error) {
	return &host.Host{}, nil
}

func (m *Manager) CanSpawn() (bool, error) {
	return true, nil
}

func (m *Manager) GetInstanceStatus(_ *host.Host) (cloud.CloudStatus, error) {
	return cloud.StatusRunning, nil
}

func (m *Manager) TerminateInstance(_ *host.Host) error {
	return nil
}

func (m *Manager) IsUp(_ *host.Host) (bool, error) {
	return true, nil
}

func (m *Manager) OnUp(_ *host.Host) error {
	return nil
}

func (m *Manager) IsSSHReachable(_ *host.Host, _ string) (bool, error) {
	return true, nil
}

func (m *Manager) GetDNSName(_ *host.Host) (string, error) {
	return "0.0.0.0", nil
}

func (*Manager) GetInstanceName(_d *distro.Distro) string {
	return "gce-" +
		fmt.Sprintf("%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

func (m *Manager) GetSSHOptions(_ *host.Host, _ string) ([]string, error) {
	return []string{}, nil
}

func (m *Manager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}
