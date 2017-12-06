// +build !go1.7

package cloud

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
)

type gceManager struct{}

type GCESettings struct{}

func (opts *ProviderSettings) Validate() error {
	return nil
}

func (m *gceManager) GetSettings() ProviderSettings {
	return &GCESettings{}
}

func (m *gceManager) Configure(_ *evergreen.Settings) error {
	return nil
}

func (m *gceManager) SpawnHost(*host.Host) (*host.Host, error) {
	return &host.Host{}, nil
}

func (m *gceManager) CanSpawn() (bool, error) {
	return true, nil
}

func (m *gceManager) GetInstanceStatus(_ *host.Host) (CloudStatus, error) {
	return StatusRunning, nil
}

func (m *gceManager) TerminateInstance(_ *host.Host) error {
	return nil
}

func (m *gceManager) IsUp(_ *host.Host) (bool, error) {
	return true, nil
}

func (m *gceManager) OnUp(_ *host.Host) error {
	return nil
}

func (m *gceManager) IsSSHReachable(_ *host.Host, _ string) (bool, error) {
	return true, nil
}

func (m *gceManager) GetDNSName(_ *host.Host) (string, error) {
	return "0.0.0.0", nil
}

func (*gceManager) GetInstanceName(_d *distro.Distro) string {
	return "gce-" +
		fmt.Sprintf("%d", rand.New(rand.NewSource(time.Now().UnixNano())).Int())
}

func (m *gceManager) GetSSHOptions(_ *host.Host, _ string) ([]string, error) {
	return []string{}, nil
}

func (m *gceManager) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}
