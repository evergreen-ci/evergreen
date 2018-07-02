package cloud

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
)

type dockerManagerMock struct {
	client dockerClientMock
}

func (m *dockerManagerMock) GetSettings() ProviderSettings {
	return &dockerSettings{}
}

func (m *dockerManagerMock) Configure(ctx context.Context, s *evergreen.Settings) {

}

func (m *dockerManagerMock) SpawnHost(ctx context.Context, h *host.Host) (*host.Host, error) {

}

func (m *dockerManagerMock) GetInstanceStatus(ctx context.Context, h *host.Host) (CloudStatus, error) {

}

func (m *dockerManagerMock) TerminateInstance(ctx context.Context, h *host.Host, user string) error {

}

func (m *dockerManagerMock) IsUp(ctx context.Context, h *host.Host) (bool, error) {

}

func (m *dockerManagerMock) OnUp(ctx context.Context, h *host.Host) error {
	return nil
}

func (m *dockerManagerMock) GetSSHOptions(h *host.Host, keyPath string) ([]string, error) {

}

// TimeTilNextPayment returns the amount of time until the next payment is due
// for the host. For Docker this is not relevant.
func (m *dockerManagerMock) TimeTilNextPayment(_ *host.Host) time.Duration {
	return time.Duration(0)
}

func (m *dockerManagerMock) GetContainers(ctx context.Context, h *host.Host) ([]string, error) {

}
