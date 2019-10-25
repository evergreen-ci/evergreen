// +build go1.7

package cloud

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"

	"github.com/stretchr/testify/suite"
)

type GCESuite struct {
	client   gceClient
	manager  *gceManager
	distro   distro.Distro
	hostOpts host.CreateOptions
	suite.Suite
}

func TestGCESuite(t *testing.T) {
	suite.Run(t, new(GCESuite))
}

func (s *GCESuite) SetupTest() {
	s.client = &gceClientMock{
		isActive:        true,
		hasAccessConfig: true,
	}
	s.manager = &gceManager{
		client: s.client,
	}
	s.distro = distro.Distro{
		Id:       "host",
		Provider: "gce",
		ProviderSettings: &map[string]interface{}{
			"instance_type": "machine",
			"image_name":    "image",
			"disk_type":     "pd-standard",
			"disk_size_gb":  10,
			"network_tags":  []string{"abc", "def", "ghi"},
		},
	}
	s.hostOpts = host.CreateOptions{}
}

func (s *GCESuite) TestValidateSettings() {
	// all required settings are provided
	settingsOk := &GCESettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsOk.Validate())

	// error when missing machine type
	settingsNoMachine := &GCESettings{
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.Error(settingsNoMachine.Validate())

	// error when missing disk type
	settingsNoDiskType := &GCESettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskSizeGB:  10,
	}
	s.Error(settingsNoDiskType.Validate())
}

func (s *GCESuite) TestValidateImageSettings() {
	settingsImageName := &GCESettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageName.Validate())

	settingsImageFamily := &GCESettings{
		MachineName: "machine",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageFamily.Validate())

	settingsOverSpecified := &GCESettings{
		MachineName: "machine",
		ImageName:   "image",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsOverSpecified.Validate())

	settingsUnderSpecified := &GCESettings{
		MachineName: "machine",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsUnderSpecified.Validate())
}

func (s *GCESuite) TestValidateMachineSettings() {
	settingsMachineName := &GCESettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsMachineName.Validate())

	settingsCustomMachine := &GCESettings{
		NumCPUs:    2,
		MemoryMB:   1024,
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.NoError(settingsCustomMachine.Validate())

	settingsOverSpecified := &GCESettings{
		MachineName: "machine",
		NumCPUs:     2,
		MemoryMB:    1024,
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsOverSpecified.Validate())

	settingsUnderSpecified := &GCESettings{
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.Error(settingsUnderSpecified.Validate())
}

func (s *GCESuite) TestConfigureAPICall() {
	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.False(mock.failInit)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings := &evergreen.Settings{}
	s.NoError(s.manager.Configure(ctx, settings))

	mock.failInit = true
	s.Error(s.manager.Configure(ctx, settings))
}

func (s *GCESuite) TestIsUpFailAPICall() {
	mock, ok := s.client.(*gceClientMock)
	s.True(ok)

	host := &host.Host{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mock.failGet = true
	_, err := s.manager.GetInstanceStatus(ctx, host)
	s.Error(err)

	active, err := s.manager.IsUp(ctx, host)
	s.Error(err)
	s.False(active)
}

func (s *GCESuite) TestIsUpStatuses() {
	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.True(mock.isActive)

	host := &host.Host{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	status, err := s.manager.GetInstanceStatus(ctx, host)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	active, err := s.manager.IsUp(ctx, host)
	s.NoError(err)
	s.True(active)

	mock.isActive = false
	status, err = s.manager.GetInstanceStatus(ctx, host)
	s.NoError(err)
	s.NotEqual(StatusRunning, status)

	active, err = s.manager.IsUp(ctx, host)
	s.NoError(err)
	s.False(active)
}

func (s *GCESuite) TestTerminateInstanceAPICall() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostA := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(ctx, hostA)
	s.NotNil(hostA)
	s.NoError(err)
	_, err = hostA.Upsert()
	s.NoError(err)

	hostB := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(ctx, hostB)
	s.NotNil(hostB)
	s.NoError(err)
	_, err = hostB.Upsert()
	s.NoError(err)

	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(ctx, hostA, evergreen.User, ""))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(ctx, hostB, evergreen.User, ""))
}

func (s *GCESuite) TestTerminateInstanceDB() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spawn the instance - check the host is not terminated in DB.
	myHost := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	myHost, err := s.manager.SpawnHost(ctx, myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert()
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(ctx, myHost, evergreen.User, "")
	s.Error(err)
}

func (s *GCESuite) TestGetDNSNameAPICall() {
	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.False(mock.failGet)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(ctx, host)
	s.NoError(err)

	mock.failGet = true
	dns, err := s.manager.GetDNSName(ctx, host)
	s.Error(err)
	s.Empty(dns)
}

func (s *GCESuite) TestGetDNSNameNetwork() {
	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.False(mock.failGet)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host := &host.Host{Id: "hostID"}
	_, err := s.manager.GetDNSName(ctx, host)
	s.NoError(err)

	mock.hasAccessConfig = false
	dns, err := s.manager.GetDNSName(ctx, host)
	s.Error(err)
	s.Empty(dns)
}

func (s *GCESuite) TestSpawnInvalidSettings() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	dProviderName := distro.Distro{Provider: evergreen.ProviderNameEc2Auto}
	h := host.NewIntent(dProviderName, dProviderName.GenerateName(), dProviderName.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)

	dSettingsNone := distro.Distro{Provider: "gce"}
	h = host.NewIntent(dSettingsNone, dSettingsNone.GenerateName(), dSettingsNone.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Nil(h)
	s.Error(err)

	dSettingsInvalid := distro.Distro{
		Provider:         "gce",
		ProviderSettings: &map[string]interface{}{"instance_type": ""},
	}
	h = host.NewIntent(dSettingsInvalid, dSettingsInvalid.GenerateName(), dSettingsInvalid.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *GCESuite) TestSpawnDuplicateHostID() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostOne, err := s.manager.SpawnHost(ctx, hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := host.NewIntent(s.distro, s.distro.GenerateName(), s.distro.Provider, s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(ctx, hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *GCESuite) TestSpawnAPICall() {
	dist := distro.Distro{
		Id:       "id",
		Provider: "gce",
		ProviderSettings: &map[string]interface{}{
			"instance_type": "machine",
			"image_name":    "image",
			"disk_type":     "pd-standard",
			"disk_size_gb":  10,
		},
	}
	opts := host.CreateOptions{}

	mock, ok := s.client.(*gceClientMock)
	s.True(ok)
	s.False(mock.failCreate)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := host.NewIntent(dist, dist.GenerateName(), dist.Provider, opts)
	h, err := s.manager.SpawnHost(ctx, h)
	s.NoError(err)
	s.NotNil(h)

	mock.failCreate = true
	h = host.NewIntent(dist, dist.GenerateName(), dist.Provider, opts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(ctx, h)
	s.Error(err)
	s.Nil(h)
}

func (s *GCESuite) TestUtilToEvgStatus() {
	s.Equal(StatusInitializing, gceToEvgStatus("PROVISIONING"))
	s.Equal(StatusInitializing, gceToEvgStatus("STAGING"))
	s.Equal(StatusRunning, gceToEvgStatus("RUNNING"))
	s.Equal(StatusStopped, gceToEvgStatus("STOPPING"))
	s.Equal(StatusTerminated, gceToEvgStatus("TERMINATED"))
	s.Equal(StatusUnknown, gceToEvgStatus("???"))
}

func (s *GCESuite) TestUtilSourceURLGenerators() {
	s.Equal("zones/zone/machineTypes/type", makeMachineType("zone", "type", 0, 0))
	s.Equal("zones/zone/machineTypes/custom-2-1024", makeMachineType("zone", "", 2, 1024))
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

func (s *GCESuite) TestUtilMakeLabels() {
	str := "!nv@lid N@m3*"
	h := &host.Host{
		Distro: distro.Distro{
			Id: str,
		},
		StartedBy:    str,
		CreationTime: time.Now(),
	}

	tags := makeLabels(h)
	r, _ := regexp.Compile("^[a-z0-9_-]*$")
	for _, v := range tags {
		s.True(r.Match([]byte(v)))
	}

	s.NotEmpty(tags["distro"])
	s.NotEmpty(tags["owner"])
	s.NotEmpty(tags["start-time"])
}
