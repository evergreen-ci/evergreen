// +build go1.7

package vsphere

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"

	"github.com/stretchr/testify/suite"
	"github.com/vmware/govmomi/vim25/types"
)

type VSphereSuite struct {
	client      client
	manager     *Manager
	suite.Suite
}

func TestVSphereSuite(t *testing.T) {
	suite.Run(t, new(VSphereSuite))
}

func (s *VSphereSuite) SetupSuite() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func (s *VSphereSuite) SetupTest() {
	s.client = &clientMock{isActive: true}
	s.manager = &Manager{
		client: s.client,
	}
}

func (s *VSphereSuite) TestConfigureAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failInit)

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(settings))

	mock.failInit = true
	s.Error(s.manager.Configure(settings))
}

func (s *VSphereSuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)

	host := &host.Host{}

	mock.failPowerState = true
	_, err := s.manager.GetInstanceStatus(host)
	s.Error(err)

	active, err := s.manager.IsUp(host)
	s.Error(err)
	s.False(active)
}

func (s *VSphereSuite) TestIsUpStatuses() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.True(mock.isActive)

	host := &host.Host{}

	status, err := s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.Equal(cloud.StatusRunning, status)

	active, err := s.manager.IsUp(host)
	s.NoError(err)
	s.True(active)

	mock.isActive = false
	status, err = s.manager.GetInstanceStatus(host)
	s.NoError(err)
	s.NotEqual(cloud.StatusRunning, status)

	active, err = s.manager.IsUp(host)
	s.NoError(err)
	s.False(active)
}

func (s *VSphereSuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failIP)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(host)
	s.NoError(err)

	mock.failIP = true
	dns, err := s.manager.GetDNSName(host)
	s.Error(err)
	s.Empty(dns)
}

func (s *VSphereSuite) TestGetSSHOptions() {
	opt := "Option"
	keyname := "key"
	host := &host.Host{
		Distro: distro.Distro{
			SSHOptions: []string{opt},
		},
	}

	opts, err := s.manager.GetSSHOptions(host, "")
	s.Error(err)
	s.Empty(opts)

	ok, err := s.manager.IsSSHReachable(host, "")
	s.Error(err)
	s.False(ok)

	opts, err = s.manager.GetSSHOptions(host, keyname)
	s.NoError(err)
	s.Equal([]string{"-i", keyname, "-o", opt}, opts)
}

func (s *VSphereSuite) TestUtilToEvgStatus() {
	poweredOn := toEvgStatus(types.VirtualMachinePowerStatePoweredOn)
	s.Equal(cloud.StatusRunning, poweredOn)

	poweredOff := toEvgStatus(types.VirtualMachinePowerStatePoweredOff)
	s.Equal(cloud.StatusStopped, poweredOff)

	suspended := toEvgStatus(types.VirtualMachinePowerStateSuspended)
	s.Equal(cloud.StatusStopped, suspended)

	unknown := toEvgStatus(types.VirtualMachinePowerState("???"))
	s.Equal(cloud.StatusUnknown, unknown)
}
