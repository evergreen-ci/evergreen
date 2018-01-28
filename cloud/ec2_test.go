package cloud

import (
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type EC2Suite struct {
	suite.Suite
	onDemandOpts    *EC2ManagerOptions
	onDemandManager CloudManager
	spotOpts        *EC2ManagerOptions
	spotManager     CloudManager
	autoOpts        *EC2ManagerOptions
	autoManager     CloudManager
	impl            *ec2Manager
}

func TestEC2Suite(t *testing.T) {
	suite.Run(t, new(EC2Suite))
}

func (s *EC2Suite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *EC2Suite) SetupTest() {
	s.Require().NoError(db.Clear(host.Collection))
	s.onDemandOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: onDemandProvider,
	}
	s.onDemandManager = NewEC2Manager(s.onDemandOpts)
	s.spotOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: spotProvider,
	}
	s.spotManager = NewEC2Manager(s.spotOpts)
	s.autoOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: autoProvider,
	}
	s.autoManager = NewEC2Manager(s.autoOpts)
	var ok bool
	s.impl, ok = s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
}

func (s *EC2Suite) TestConstructor() {
	s.Implements((*CloudManager)(nil), NewEC2Manager(s.onDemandOpts))
}

func (s *EC2Suite) TestValidateProviderSettings() {
	p := &EC2ProviderSettings{
		AMI:           "ami",
		InstanceType:  "type",
		SecurityGroup: "sg-123456",
		KeyName:       "keyName",
	}
	s.Nil(p.Validate())
	p.AMI = ""
	s.Error(p.Validate())
	p.AMI = "ami"

	s.Nil(p.Validate())
	p.InstanceType = ""
	s.Error(p.Validate())
	p.InstanceType = "type"

	s.Nil(p.Validate())
	p.SecurityGroup = ""
	s.Error(p.Validate())
	p.SecurityGroup = "sg-123456"

	s.Nil(p.Validate())
	p.KeyName = ""
	s.Error(p.Validate())
	p.KeyName = "keyName"

	p.BidPrice = -1
	s.Error(p.Validate())
	p.BidPrice = 1
	s.NoError(p.Validate())

	p.IsVpc = true
	s.Error(p.Validate())
	p.SubnetId = "subnet-123456"
	s.NoError(p.Validate())
}

func (s *EC2Suite) TestMakeDeviceMappings() {
	validMount := MountPoint{
		DeviceName:  "device",
		VirtualName: "virtual",
	}

	m := []MountPoint{}
	b, err := makeBlockDeviceMappings(m)
	s.NoError(err)
	s.Len(b, 0)

	noDeviceName := validMount
	noDeviceName.DeviceName = ""
	m = []MountPoint{validMount, noDeviceName}
	b, err = makeBlockDeviceMappings(m)
	s.Nil(b)
	s.Error(err)

	noVirtualName := validMount
	noVirtualName.VirtualName = ""
	m = []MountPoint{validMount, noVirtualName}
	b, err = makeBlockDeviceMappings(m)
	s.Nil(b)
	s.Error(err)

	anotherMount := validMount
	anotherMount.DeviceName = "anotherDeviceName"
	anotherMount.VirtualName = "anotherVirtualName"
	m = []MountPoint{validMount, anotherMount}
	b, err = makeBlockDeviceMappings(m)
	s.Len(b, 2)
	s.Equal("device", *b[0].DeviceName)
	s.Equal("virtual", *b[0].VirtualName)
	s.Equal("anotherDeviceName", *b[1].DeviceName)
	s.Equal("anotherVirtualName", *b[1].VirtualName)
	s.NoError(err)
}

func (s *EC2Suite) TestGetSettings() {
	s.Equal(&EC2ProviderSettings{}, s.onDemandManager.GetSettings())
}

func (s *EC2Suite) TestConfigure() {
	settings := &evergreen.Settings{}
	err := s.onDemandManager.Configure(settings)
	s.Error(err)

	settings.Providers.AWS.Id = "id"
	err = s.onDemandManager.Configure(settings)
	s.Error(err)

	settings.Providers.AWS.Id = ""
	settings.Providers.AWS.Secret = "secret"
	err = s.onDemandManager.Configure(settings)
	s.Error(err)

	settings.Providers.AWS.Id = "id"
	err = s.onDemandManager.Configure(settings)
	s.NoError(err)
	ec2m := s.onDemandManager.(*ec2Manager)
	creds, err := ec2m.credentials.Get()
	s.NoError(err)
	s.Equal("id", creds.AccessKeyID)
	s.Equal("secret", creds.SecretAccessKey)
}

func (s *EC2Suite) TestSpawnHostInvalidInput() {
	h := &host.Host{
		Distro: distro.Distro{
			Provider: "foo",
			Id:       "id",
		},
	}

	spawned, err := s.onDemandManager.SpawnHost(h)
	s.Nil(spawned)
	s.Error(err)
	s.EqualError(err, "Can't spawn instance for distro id: provider is foo")
}

func (s *EC2Suite) TestSpawnHostClassicOnDemand() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "keyName",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group": "sg-123456",
		"subnet_id":      "subnet-123456",
	}

	_, err := s.onDemandManager.SpawnHost(h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.Equal("instanceType", *runInput.InstanceType)
	s.Equal("keyName", *runInput.KeyName)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Equal("sg-123456", *runInput.SecurityGroups[0])
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SubnetId)
	describeInput := *mock.DescribeInstancesInput
	s.Equal("instance_id", *describeInput.InstanceIds[0])
	tagsInput := *mock.CreateTagsInput
	s.Equal("instance_id", *tagsInput.Resources[0])
	s.Len(tagsInput.Tags, 8)
	var foundInstanceName bool
	var foundDistroID bool
	for _, tag := range tagsInput.Tags {
		if *tag.Key == "name" {
			foundInstanceName = true
			s.Equal(*tag.Value, "instance_id")
		}
		if *tag.Key == "distro" {
			foundDistroID = true
			s.Equal(*tag.Value, "distro_id")
		}
	}
	s.True(foundInstanceName)
	s.True(foundDistroID)
}

func (s *EC2Suite) TestSpawnHostVPCOnDemand() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "keyName",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group": "sg-123456",
		"subnet_id":      "subnet-123456",
		"is_vpc":         true,
	}

	_, err := s.onDemandManager.SpawnHost(h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.Equal("instanceType", *runInput.InstanceType)
	s.Equal("keyName", *runInput.KeyName)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SecurityGroups)
	s.Nil(runInput.SubnetId)
	describeInput := *mock.DescribeInstancesInput
	s.Equal("instance_id", *describeInput.InstanceIds[0])
	tagsInput := *mock.CreateTagsInput
	s.Equal("instance_id", *tagsInput.Resources[0])
	s.Len(tagsInput.Tags, 8)
	var foundInstanceName bool
	var foundDistroID bool
	for _, tag := range tagsInput.Tags {
		if *tag.Key == "name" {
			foundInstanceName = true
			s.Equal(*tag.Value, "instance_id")
		}
		if *tag.Key == "distro" {
			foundDistroID = true
			s.Equal(*tag.Value, "distro_id")
		}
	}
	s.True(foundInstanceName)
	s.True(foundDistroID)
}

func (s *EC2Suite) TestSpawnHostClassicSpot() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "keyName",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group": "sg-123456",
		"subnet_id":      "subnet-123456",
	}

	_, err := s.spotManager.SpawnHost(h)
	s.NoError(err)

	manager, ok := s.spotManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	requestInput := *mock.RequestSpotInstancesInput
	s.Equal("ami", *requestInput.LaunchSpecification.ImageId)
	s.Equal("instanceType", *requestInput.LaunchSpecification.InstanceType)
	s.Equal("keyName", *requestInput.LaunchSpecification.KeyName)
	s.Equal("virtual", *requestInput.LaunchSpecification.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *requestInput.LaunchSpecification.BlockDeviceMappings[0].DeviceName)
	s.Equal("sg-123456", *requestInput.LaunchSpecification.SecurityGroups[0])
	s.Nil(requestInput.LaunchSpecification.SecurityGroupIds)
	s.Nil(requestInput.LaunchSpecification.SubnetId)
	tagsInput := *mock.CreateTagsInput
	s.Equal("instance_id", *tagsInput.Resources[0])
	s.Len(tagsInput.Tags, 8)
	var foundInstanceName bool
	var foundDistroID bool
	for _, tag := range tagsInput.Tags {
		if *tag.Key == "name" {
			foundInstanceName = true
			s.Equal(*tag.Value, "instance_id")
		}
		if *tag.Key == "distro" {
			foundDistroID = true
			s.Equal(*tag.Value, "distro_id")
		}
	}
	s.True(foundInstanceName)
	s.True(foundDistroID)
}

func (s *EC2Suite) TestSpawnHostVPCSpot() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "keyName",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group": "sg-123456",
		"subnet_id":      "subnet-123456",
		"is_vpc":         true,
	}

	_, err := s.spotManager.SpawnHost(h)
	s.NoError(err)

	manager, ok := s.spotManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	requestInput := *mock.RequestSpotInstancesInput
	s.Equal("ami", *requestInput.LaunchSpecification.ImageId)
	s.Equal("instanceType", *requestInput.LaunchSpecification.InstanceType)
	s.Equal("keyName", *requestInput.LaunchSpecification.KeyName)
	s.Equal("virtual", *requestInput.LaunchSpecification.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *requestInput.LaunchSpecification.BlockDeviceMappings[0].DeviceName)
	s.Nil(requestInput.LaunchSpecification.SecurityGroupIds)
	s.Nil(requestInput.LaunchSpecification.SecurityGroups)
	s.Nil(requestInput.LaunchSpecification.SubnetId)
	tagsInput := *mock.CreateTagsInput
	s.Equal("instance_id", *tagsInput.Resources[0])
	s.Len(tagsInput.Tags, 8)
	var foundInstanceName bool
	var foundDistroID bool
	for _, tag := range tagsInput.Tags {
		if *tag.Key == "name" {
			foundInstanceName = true
			s.Equal(*tag.Value, "instance_id")
		}
		if *tag.Key == "distro" {
			foundDistroID = true
			s.Equal(*tag.Value, "distro_id")
		}
	}
	s.True(foundInstanceName)
	s.True(foundDistroID)
}

func (s *EC2Suite) TestCanSpawn() {
	can, err := s.onDemandManager.CanSpawn()
	s.True(can)
	s.NoError(err)
}

func (s *EC2Suite) TestGetInstanceStatus() {
	h := &host.Host{}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	status, err := s.onDemandManager.GetInstanceStatus(h)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	status, err = s.onDemandManager.GetInstanceStatus(h)
	s.NoError(err)
	s.Equal(StatusRunning, status)
}

func (s *EC2Suite) TestTerminateInstance() {
	h := &host.Host{Id: "host_id"}
	s.NoError(h.Insert())
	s.NoError(s.onDemandManager.TerminateInstance(h, evergreen.User))
	found, err := host.FindOne(host.ById("host_id"))
	s.Equal(evergreen.HostTerminated, found.Status)
	s.NoError(err)
}

func (s *EC2Suite) TestIsUp() {
	h := &host.Host{
		Distro: distro.Distro{},
	}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	up, err := s.onDemandManager.IsUp(h)
	s.True(up)
	s.NoError(err)

	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	up, err = s.onDemandManager.IsUp(h)
	s.True(up)
	s.NoError(err)

}

func (s *EC2Suite) TestOnUp() {
	s.NoError(s.onDemandManager.OnUp(&host.Host{}))
}

func (s *EC2Suite) TestGetDNSName() {
	dns, err := s.onDemandManager.GetDNSName(&host.Host{})
	s.Equal("public_dns_name", dns)
	s.NoError(err)
}

func (s *EC2Suite) TestGetSSHOptionsEmptyKey() {
	opts, err := s.onDemandManager.GetSSHOptions(&host.Host{}, "")
	s.Nil(opts)
	s.Error(err)
}

func (s *EC2Suite) TestGetSSHOptions() {
	h := &host.Host{
		Distro: distro.Distro{
			SSHOptions: []string{
				"foo",
				"bar",
			},
		},
	}
	opts, err := s.onDemandManager.GetSSHOptions(h, "key")
	s.Equal([]string{"-i", "key", "-o", "foo", "-o", "bar", "-o", "UserKnownHostsFile=/dev/null"}, opts)
	s.NoError(err)
}

func (s *EC2Suite) TestTimeTilNextPaymentLinux() {
	h := &host.Host{
		Distro: distro.Distro{
			Arch: "linux",
		},
	}
	s.Equal(time.Second, s.onDemandManager.TimeTilNextPayment(h))
}

func (s *EC2Suite) TestTimeTilNextPaymentWindows() {
	now := time.Now()
	thirtyMinutesAgo := now.Add(-30 * time.Minute)
	h := &host.Host{
		Distro: distro.Distro{
			Arch: "windows",
		},
		CreationTime: thirtyMinutesAgo,
		StartTime:    thirtyMinutesAgo.Add(time.Minute),
	}
	s.InDelta(31*time.Minute, s.onDemandManager.TimeTilNextPayment(h), float64(time.Millisecond))
}

func (s *EC2Suite) TestGetInstanceName() {
	id := s.onDemandManager.GetInstanceName(&distro.Distro{Id: "foo"})
	s.True(strings.HasPrefix(id, "evg-foo-"))
}

func (s *EC2Suite) TestGetProvider() {
	h := &host.Host{
		Distro: distro.Distro{
			Arch: "Linux/Unix",
		},
	}
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{
			os:       "Linux",
			instance: "instance",
			region:   "US East (N. Virginia)",
		}: 23.2,
	}
	ec2Settings := &EC2ProviderSettings{
		InstanceType: "instance",
		IsVpc:        true,
		SubnetId:     "subnet-123456",
		VpcName:      "vpc_name",
	}
	manager, ok := s.autoManager.(*ec2Manager)
	s.True(ok)
	provider, err := manager.getProvider(h, ec2Settings)
	s.NoError(err)
	s.Equal(spotProvider, provider)
	// subnet should be set based on vpc name
	s.Equal("subnet-654321", ec2Settings.SubnetId)
	s.Equal(h.Distro.Provider, evergreen.ProviderNameEc2Spot)

	h.UserHost = true
	provider, err = manager.getProvider(h, ec2Settings)
	s.NoError(err)
	s.Equal(onDemandProvider, provider)
}
