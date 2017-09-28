// +build go1.7

package gce

import (
	"regexp"
	"strings"
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
	client   client
	manager  *Manager
	distro   *distro.Distro
	hostOpts cloud.HostOptions
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
	s.manager = &Manager{
		client: s.client,
	}
	s.distro = &distro.Distro{
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
	s.hostOpts = cloud.HostOptions{}
}

func (s *GCESuite) TestValidateSettings() {
	// all required settings are provided
	settingsOk := &ProviderSettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsOk.Validate())

	// error when missing machine type
	settingsNoMachine := &ProviderSettings{
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.Error(settingsNoMachine.Validate())

	// error when missing disk type
	settingsNoDiskType := &ProviderSettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskSizeGB:  10,
	}
	s.Error(settingsNoDiskType.Validate())
}

func (s *GCESuite) TestValidateImageSettings() {
	settingsImageName := &ProviderSettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageName.Validate())

	settingsImageFamily := &ProviderSettings{
		MachineName: "machine",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsImageFamily.Validate())

	settingsOverSpecified := &ProviderSettings{
		MachineName: "machine",
		ImageName:   "image",
		ImageFamily: "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsOverSpecified.Validate())

	settingsUnderSpecified := &ProviderSettings{
		MachineName: "machine",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsUnderSpecified.Validate())
}

func (s *GCESuite) TestValidateMachineSettings() {
	settingsMachineName := &ProviderSettings{
		MachineName: "machine",
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.NoError(settingsMachineName.Validate())

	settingsCustomMachine := &ProviderSettings{
		NumCPUs:    2,
		MemoryMB:   1024,
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
	}
	s.NoError(settingsCustomMachine.Validate())

	settingsOverSpecified := &ProviderSettings{
		MachineName: "machine",
		NumCPUs:     2,
		MemoryMB:    1024,
		ImageName:   "image",
		DiskType:    "pd-standard",
		DiskSizeGB:  10,
	}
	s.Error(settingsOverSpecified.Validate())

	settingsUnderSpecified := &ProviderSettings{
		ImageName:  "image",
		DiskType:   "pd-standard",
		DiskSizeGB: 10,
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
	hostA := cloud.NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostA, err := s.manager.SpawnHost(hostA)
	s.NotNil(hostA)
	s.NoError(err)
	_, err = hostA.Upsert()
	s.NoError(err)

	hostB := cloud.NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostB, err = s.manager.SpawnHost(hostB)
	s.NotNil(hostB)
	s.NoError(err)
	_, err = hostB.Upsert()
	s.NoError(err)

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failDelete)

	s.NoError(s.manager.TerminateInstance(hostA))

	mock.failDelete = true
	s.Error(s.manager.TerminateInstance(hostB))
}

func (s *GCESuite) TestTerminateInstanceDB() {
	// Spawn the instance - check the host is not terminated in DB.
	myHost := cloud.NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	myHost, err := s.manager.SpawnHost(myHost)
	s.NotNil(myHost)
	s.NoError(err)
	_, err = myHost.Upsert()
	s.NoError(err)

	dbHost, err := host.FindOne(host.ById(myHost.Id))
	s.NotEqual(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate the instance - check the host is terminated in DB.
	err = s.manager.TerminateInstance(myHost)
	s.NoError(err)

	dbHost, err = host.FindOne(host.ById(myHost.Id))
	s.Equal(dbHost.Status, evergreen.HostTerminated)
	s.NoError(err)

	// Terminate again - check we cannot remove twice.
	err = s.manager.TerminateInstance(myHost)
	s.Error(err)
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

func (s *GCESuite) TestSpawnInvalidSettings() {
	var err error
	dProviderName := &distro.Distro{Provider: "ec2"}
	h := cloud.NewIntent(*dProviderName, s.manager.GetInstanceName(dProviderName), dProviderName.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(h)
	s.Error(err)
	s.Nil(h)

	dSettingsNone := &distro.Distro{Provider: "gce"}
	h = cloud.NewIntent(*dSettingsNone, s.manager.GetInstanceName(dSettingsNone), dSettingsNone.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(h)
	s.Nil(h)
	s.Error(err)

	dSettingsInvalid := &distro.Distro{
		Provider:         "gce",
		ProviderSettings: &map[string]interface{}{"instance_type": ""},
	}
	h = cloud.NewIntent(*dSettingsInvalid, s.manager.GetInstanceName(dSettingsInvalid), dSettingsInvalid.Provider, s.hostOpts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(h)
	s.Error(err)
	s.Nil(h)
}

func (s *GCESuite) TestSpawnDuplicateHostID() {
	// SpawnInstance should generate a unique ID for each instance, even
	// when using the same distro. Otherwise the DB would return an error.
	hostOne := cloud.NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostOne, err := s.manager.SpawnHost(hostOne)
	s.NoError(err)
	s.NotNil(hostOne)

	hostTwo := cloud.NewIntent(*s.distro, s.manager.GetInstanceName(s.distro), s.distro.Provider, s.hostOpts)
	hostTwo, err = s.manager.SpawnHost(hostTwo)
	s.NoError(err)
	s.NotNil(hostTwo)
}

func (s *GCESuite) TestSpawnAPICall() {
	dist := &distro.Distro{
		Id:       "id",
		Provider: "gce",
		ProviderSettings: &map[string]interface{}{
			"instance_type": "machine",
			"image_name":    "image",
			"disk_type":     "pd-standard",
			"disk_size_gb":  10,
		},
	}
	opts := cloud.HostOptions{}

	mock, ok := s.client.(*clientMock)
	s.True(ok)
	s.False(mock.failCreate)

	h := cloud.NewIntent(*dist, s.manager.GetInstanceName(dist), dist.Provider, opts)
	h, err := s.manager.SpawnHost(h)
	s.NoError(err)
	s.NotNil(h)

	mock.failCreate = true
	h = cloud.NewIntent(*dist, s.manager.GetInstanceName(dist), dist.Provider, opts)
	s.NotNil(h)
	h, err = s.manager.SpawnHost(h)
	s.Error(err)
	s.Nil(h)
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

func (s *GCESuite) TestUtilGenerateName() {
	r, _ := regexp.Compile("(?:[a-z](?:[-a-z0-9]{0,61}[a-z0-9])?)")
	d := &distro.Distro{Id: "name"}

	nameA := generateName(d)
	nameB := generateName(d)
	s.True(r.Match([]byte(nameA)))
	s.True(r.Match([]byte(nameB)))
	s.NotEqual(nameA, nameB)

	d.Id = "!nv@lid N@m3*"
	invalidChars := generateName(d)
	s.True(r.Match([]byte(invalidChars)))

	d.Id = strings.Repeat("abc", 10)
	tooManyChars := generateName(d)
	s.True(r.Match([]byte(tooManyChars)))
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
