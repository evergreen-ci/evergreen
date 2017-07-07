package gce

import (
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"

	"github.com/stretchr/testify/suite"
)

type GCESuite struct {
	client  client
	keyname string
	manager *Manager
	suite.Suite
}

func TestGCESuite(t *testing.T) {
	suite.Run(t, new(GCESuite))
}

func (s *GCESuite) SetupSuite() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func (s *GCESuite) SetupTest() {
	s.client = &clientMock{
		isActive:        true,
		hasAccessConfig: true,
	}
	s.keyname = "key"

	s.manager = &Manager{
		client: s.client,
	}
}

func (s *GCESuite) TestValidateSettings() {
	settingsOk := &ProviderSettings{
		MachineType: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsOk.Validate())

	settingsNoMachine := &ProviderSettings{
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.Error(settingsNoMachine.Validate())

	settingsNoDiskType := &ProviderSettings{
		MachineType: "machine",
		ImageName:   "image",
		DiskSizeGB:  10,
	}
	s.Error(settingsNoDiskType.Validate())
}

func (s *GCESuite) TestValidateImageSettings() {
	settingsImageName := &ProviderSettings{
		MachineType: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageName.Validate())

	settingsImageFamily := &ProviderSettings{
		MachineType: "machine",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageFamily.Validate())

	settingsOverSpecified := &ProviderSettings{
		MachineType: "machine",
		ImageName:   "image",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsOverSpecified.Validate())

	settingsUnderSpecified := &ProviderSettings{
		MachineType: "machine",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsUnderSpecified.Validate())
}

func (s *GCESuite) TestConfigureAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failInit)

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(settings))

	mock.failInit = true
	s.Error(s.manager.Configure(settings))
}

func (s *GCESuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)

	host := &host.Host{}

	mock.failGet = true
	_, err := s.manager.GetInstanceStatus(host)
	s.Error(err)

	active, err := s.manager.IsUp(host)
	s.Error(err)
	s.False(active)
}

func (s *GCESuite) TestIsUpStatuses() {
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

func (s *GCESuite) TestTerminateInstanceAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failDelete)

	host := &host.Host{Id: "hostID"}
	s.NoError(s.manager.TerminateInstance(host))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(host))
}

func (s *GCESuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failGet)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(host)
	s.NoError(err)

	mock.failGet = true
	dns, err := s.manager.GetDNSName(host)
	s.Error(err)
	s.Empty(dns)
}

func (s *GCESuite) TestGetDNSNameNetwork() {
	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failGet)

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(host)
	s.NoError(err)

	mock.hasAccessConfig = false
	dns, err := s.manager.GetDNSName(host)
	s.Error(err)
	s.Empty(dns)
}

func (s *GCESuite) TestGetSSHOptions() {
	opt := "Option"
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

	opts, err = s.manager.GetSSHOptions(host, s.keyname)
	s.NoError(err)
	s.Equal([]string{"-i", s.keyname, "-o", opt}, opts)
}

func (s *GCESuite) TestSpawnInvalidSettings() {
	hostOpts := cloud.HostOptions{}

	dProviderName := &distro.Distro{Provider: "ec2"}
	host, err := s.manager.SpawnInstance(dProviderName, hostOpts)
	s.Error(err)
	s.Nil(host)

	dSettingsNone := &distro.Distro{Provider: "gce"}
	host, err = s.manager.SpawnInstance(dSettingsNone, hostOpts)
	s.Error(err)
	s.Nil(host)

	dSettingsInvalid := &distro.Distro{
		Provider:         "gce",
		ProviderSettings: &map[string]interface{}{"machine_type": ""},
	}
	host, err = s.manager.SpawnInstance(dSettingsInvalid, hostOpts)
	s.Error(err)
	s.Nil(host)
}

func (s *GCESuite) TestSpawnDuplicateHostID() {
	dist := &distro.Distro{
		Id:       "host",
		Provider: "gce",
		ProviderSettings: &map[string]interface{}{
			"machine_type": "machine",
			"image_name":   "image",
			"disk_type":    "pd-standard",
			"disk_size_gb": 10,
		},
	}
	opts := cloud.HostOptions{}

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne, err := s.manager.SpawnInstance(dist, opts)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo, err := s.manager.SpawnInstance(dist, opts)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *GCESuite) TestSpawnAPICall() {
	dist := &distro.Distro{
		Id:       "id",
		Provider: "gce",
		ProviderSettings: &map[string]interface{}{
			"machine_type": "machine",
			"image_name":   "image",
			"disk_type":    "pd-standard",
			"disk_size_gb": 10,
		},
	}
	opts := cloud.HostOptions{}

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failCreate)

	host, err := s.manager.SpawnInstance(dist, opts)
	s.NoError(err)
	s.NotNil(host)

	mock.failCreate = true
	host, err = s.manager.SpawnInstance(dist, opts)
	s.Error(err)
	s.Nil(host)
}

func (s *GCESuite) TestUtilToEvgStatus() {
	s.Equal(cloud.StatusInitializing, toEvgStatus("PROVISIONING"))
	s.Equal(cloud.StatusInitializing, toEvgStatus("STAGING"))
	s.Equal(cloud.StatusRunning, toEvgStatus("RUNNING"))
	s.Equal(cloud.StatusStopped, toEvgStatus("STOPPING"))
	s.Equal(cloud.StatusTerminated, toEvgStatus("TERMINATED"))
	s.Equal(cloud.StatusUnknown, toEvgStatus("???"))
}

func (s *GCESuite) TestUtilSourceURLGenerators() {
	s.Equal("zones/zone/machineTypes/type", makeMachineType("zone", "type"))
	s.Equal("zones/zone/diskTypes/type", makeDiskType("zone", "type"))
	s.Equal("global/images/family/family", makeImageFromFamily("family"))
	s.Equal("global/images/name", makeImage("name"))
}

func (s *GCESuite) TestUtilSSHKeyFormatters() {
	key := sshKey{Username: "user", PublicKey: "key"}
	s.Equal("user:key", key.String())

	keys := sshKeyGroup{
		key,
		sshKey{Username: "user1", PublicKey: "key1"},
		sshKey{Username: "user2", PublicKey: "key2"},
	}
	s.Equal("user:key\nuser1:key1\nuser2:key2", keys.String())
}

func (s *GCESuite) TestUtilMakeTags() {
	str := "!nv@lid N@m3*"
	host := &host.Host{
		Distro: distro.Distro{
			Id: str,
		},
		StartedBy: str,
		CreationTime: time.Now(),
	}

	tags := makeTags(host)
	r, _ := regexp.Compile("^[a-z0-9_-]*$")
	for _, v := range tags {
		s.True(r.Match([]byte(v)))
	}

	s.NotEmpty(tags["distro"])
	s.NotEmpty(tags["owner"])
	s.NotEmpty(tags["start-time"])
}
