package cloud

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/suite"
)

var (
	someUserData         = "some user data"
	base64OfSomeUserData = base64.StdEncoding.EncodeToString([]byte(someUserData))
)

type EC2Suite struct {
	suite.Suite
	onDemandOpts    *EC2ManagerOptions
	onDemandManager Manager
	spotOpts        *EC2ManagerOptions
	spotManager     Manager
	autoOpts        *EC2ManagerOptions
	autoManager     Manager
	impl            *ec2Manager
	h               *host.Host
}

func TestEC2Suite(t *testing.T) {
	suite.Run(t, new(EC2Suite))
}

func (s *EC2Suite) SetupTest() {
	s.Require().NoError(db.ClearCollections(host.Collection, task.Collection, model.ProjectVarsCollection))
	s.onDemandOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: onDemandProvider,
	}
	s.onDemandManager = NewEC2Manager(s.onDemandOpts)
	_ = s.onDemandManager.Configure(context.Background(), &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	s.spotOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: spotProvider,
	}
	s.spotManager = NewEC2Manager(s.spotOpts)
	_ = s.spotManager.Configure(context.Background(), &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	s.autoOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: autoProvider,
	}
	s.autoManager = NewEC2Manager(s.autoOpts)
	_ = s.autoManager.Configure(context.Background(), &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	var ok bool
	s.impl, ok = s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)

	s.h = &host.Host{
		Id: "h1",
		Distro: distro.Distro{
			ProviderSettings: &map[string]interface{}{
				"key_name":           "key",
				"aws_access_key_id":  "key_id",
				"ami":                "ami",
				"instance_type":      "instance",
				"security_group_ids": []string{"abcdef"},
				"bid_price":          float64(0.001),
			},
			Provider: evergreen.ProviderNameEc2OnDemand,
		},
	}
}

func (s *EC2Suite) TestConstructor() {
	s.Implements((*Manager)(nil), NewEC2Manager(s.onDemandOpts))
	s.Implements((*BatchManager)(nil), NewEC2Manager(s.onDemandOpts))
}

func (s *EC2Suite) TestValidateProviderSettings() {
	p := &EC2ProviderSettings{
		AMI:              "ami",
		InstanceType:     "type",
		SecurityGroupIDs: []string{"sg-123456"},
		KeyName:          "keyName",
	}
	s.NoError(p.Validate())
	p.AMI = ""
	s.Error(p.Validate())
	p.AMI = "ami"

	s.NoError(p.Validate())
	p.InstanceType = ""
	s.Error(p.Validate())
	p.InstanceType = "type"

	s.NoError(p.Validate())
	p.SecurityGroupIDs = nil
	s.Error(p.Validate())
	p.SecurityGroupIDs = []string{"sg-123456"}

	s.NoError(p.Validate())
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

	ebsMount := MountPoint{
		DeviceName: "device",
		Size:       10,
		Iops:       100,
		SnapshotID: "snapshot-1",
	}
	b, err = makeBlockDeviceMappings([]MountPoint{ebsMount})
	s.NoError(err)
	s.Len(b, 1)
	s.Equal("device", *b[0].DeviceName)
	s.Equal(int64(10), *b[0].Ebs.VolumeSize)
	s.Equal(int64(100), *b[0].Ebs.Iops)
	s.Equal("snapshot-1", *b[0].Ebs.SnapshotId)
}

func (s *EC2Suite) TestMakeDeviceMappingsTemplate() {
	validMount := MountPoint{
		DeviceName:  "device",
		VirtualName: "virtual",
	}

	m := []MountPoint{}
	b, err := makeBlockDeviceMappingsTemplate(m)
	s.NoError(err)
	s.Len(b, 0)

	noDeviceName := validMount
	noDeviceName.DeviceName = ""
	m = []MountPoint{validMount, noDeviceName}
	b, err = makeBlockDeviceMappingsTemplate(m)
	s.Nil(b)
	s.Error(err)

	noVirtualName := validMount
	noVirtualName.VirtualName = ""
	m = []MountPoint{validMount, noVirtualName}
	b, err = makeBlockDeviceMappingsTemplate(m)
	s.Nil(b)
	s.Error(err)

	anotherMount := validMount
	anotherMount.DeviceName = "anotherDeviceName"
	anotherMount.VirtualName = "anotherVirtualName"
	m = []MountPoint{validMount, anotherMount}
	b, err = makeBlockDeviceMappingsTemplate(m)
	s.Len(b, 2)
	s.Equal("device", *b[0].DeviceName)
	s.Equal("virtual", *b[0].VirtualName)
	s.Equal("anotherDeviceName", *b[1].DeviceName)
	s.Equal("anotherVirtualName", *b[1].VirtualName)
	s.NoError(err)

	ebsMount := MountPoint{
		DeviceName: "device",
		Size:       10,
		Iops:       100,
		SnapshotID: "snapshot-1",
	}
	b, err = makeBlockDeviceMappingsTemplate([]MountPoint{ebsMount})
	s.NoError(err)
	s.Len(b, 1)
	s.Equal("device", *b[0].DeviceName)
	s.Equal(int64(10), *b[0].Ebs.VolumeSize)
	s.Equal(int64(100), *b[0].Ebs.Iops)
	s.Equal("snapshot-1", *b[0].Ebs.SnapshotId)
}

func (s *EC2Suite) TestGetSettings() {
	s.Equal(&EC2ProviderSettings{}, s.onDemandManager.GetSettings())
}

func (s *EC2Suite) TestConfigure() {
	settings := &evergreen.Settings{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := s.onDemandManager.Configure(ctx, settings)
	s.Error(err)

	settings.Providers.AWS.EC2Key = "id"
	err = s.onDemandManager.Configure(ctx, settings)
	s.Error(err)

	settings.Providers.AWS.EC2Key = ""
	settings.Providers.AWS.EC2Secret = "secret"
	err = s.onDemandManager.Configure(ctx, settings)
	s.Error(err)

	settings.Providers.AWS.EC2Key = "id"
	err = s.onDemandManager.Configure(ctx, settings)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	spawned, err := s.onDemandManager.SpawnHost(ctx, h)
	s.Nil(spawned)
	s.Error(err)
	s.EqualError(err, "Can't spawn instance for distro id: provider is foo")
}

func (s *EC2Suite) TestSpawnHostClassicOnDemand() {
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{"Linux", "instanceType", "US East (N. Virginia)"}: .1,
	}
	s.h.Distro.Id = "distro_id"
	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	s.h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "keyName",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"user_data":          someUserData,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, s.h)
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
	s.Equal(base64OfSomeUserData, *runInput.UserData)

	// Compute cost is cached in the host
	s.Equal(.1, s.h.ComputeCostPerHour)
}

func (s *EC2Suite) TestSpawnHostVPCOnDemand() {
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{"Linux", "instanceType", "US East (N. Virginia)"}: .1,
	}
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
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"is_vpc":             true,
		"user_data":          someUserData,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
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
	s.Equal(base64OfSomeUserData, *runInput.UserData)

	// Compute cost is cached in the host
	s.Equal(.1, h.ComputeCostPerHour)
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
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"user_data":          someUserData,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := s.spotManager.SpawnHost(ctx, h)
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
	s.Equal(base64OfSomeUserData, *requestInput.LaunchSpecification.UserData)

	// Compute cost is cached
	s.Equal(1.0, h.ComputeCostPerHour)
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
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"is_vpc":             true,
		"user_data":          someUserData,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, err := s.spotManager.SpawnHost(ctx, h)
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
	s.Equal(base64OfSomeUserData, *requestInput.LaunchSpecification.UserData)

	// Compute cost is cached
	s.Equal(1.0, h.ComputeCostPerHour)
}

func (s *EC2Suite) TestNoKeyAndNotSpawnHostForTaskShouldFail() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"is_vpc":             true,
		"user_data":          someUserData,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
	s.Error(err)
}

func (s *EC2Suite) TestSpawnHostForTask() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettings = &map[string]interface{}{
		"ami":           "ami",
		"instance_type": "instanceType",
		"key_name":      "",
		"mount_points": []map[string]string{
			map[string]string{"device_name": "device", "virtual_name": "virtual"},
		},
		"security_group_ids": []string{"sg-123456"},
		"subnet_id":          "subnet-123456",
		"is_vpc":             true,
		"user_data":          someUserData,
	}

	project := "example_project"
	t := &task.Task{
		Id:      "task_1",
		Project: project,
	}
	h.SpawnOptions.TaskID = "task_1"
	h.StartedBy = "task_1"
	h.SpawnOptions.SpawnedByTask = true
	s.Require().NoError(t.Insert())
	newVars := &model.ProjectVars{
		Id: project,
		Vars: map[string]string{
			model.ProjectAWSSSHKeyName:  "evg_auto_example_project",
			model.ProjectAWSSSHKeyValue: "key_material",
		},
	}
	s.Require().NoError(newVars.Insert())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.Equal("instanceType", *runInput.InstanceType)
	s.Equal("evg_auto_evergreen", *runInput.KeyName)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SecurityGroups)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}
func (s *EC2Suite) TestGetInstanceStatus() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Require().NoError(s.h.Insert())

	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	status, err := s.onDemandManager.GetInstanceStatus(ctx, s.h)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	// instance information is cached in the host
	s.Equal("us-east-1a", s.h.Zone)
	s.True(s.h.StartTime.Equal(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)))
	s.Equal("public_dns_name", s.h.Host)
	s.Equal([]string{"volume_id"}, s.h.VolumeIDs)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)
	volumesInput := *mock.DescribeVolumesInput
	s.Len(volumesInput.VolumeIds, 1)
	s.Equal("volume_id", *volumesInput.VolumeIds[0])

	s.h.Distro.Provider = evergreen.ProviderNameEc2Spot
	s.h.Id = "instance_id"
	status, err = s.onDemandManager.GetInstanceStatus(ctx, s.h)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	s.Equal("instance_id", s.h.ExternalIdentifier)
}

func (s *EC2Suite) TestTerminateInstance() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.h.Insert())
	s.NoError(s.onDemandManager.TerminateInstance(ctx, s.h, evergreen.User))
	found, err := host.FindOne(host.ById("h1"))
	s.Equal(evergreen.HostTerminated, found.Status)
	s.NoError(err)
}

func (s *EC2Suite) TestIsUp() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	up, err := s.onDemandManager.IsUp(ctx, s.h)
	s.True(up)
	s.NoError(err)

	s.h.Distro.Provider = evergreen.ProviderNameEc2Spot
	up, err = s.onDemandManager.IsUp(ctx, s.h)
	s.True(up)
	s.NoError(err)
}

func (s *EC2Suite) TestOnUp() {
	s.h.VolumeIDs = []string{"volume_id"}

	s.NoError(s.onDemandManager.OnUp(context.Background(), s.h))
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)
	s.Nil(mock.DescribeVolumesInput)

	s.Len(mock.CreateTagsInput.Resources, 2)
	s.Equal(s.h.Id, *mock.CreateTagsInput.Resources[0])
	s.Equal("volume_id", *mock.CreateTagsInput.Resources[1])
}

func (s *EC2Suite) TestGetDNSName() {
	s.h.Host = "public_dns_name"
	dns, err := s.onDemandManager.GetDNSName(context.Background(), s.h)
	s.Equal("public_dns_name", dns)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)
	s.Nil(mock.DescribeInstancesInput)
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
	d := distro.Distro{Id: "foo"}
	id := d.GenerateName()
	s.True(strings.HasPrefix(id, "evg-foo-"))
}

func (s *EC2Suite) TestGetProvider() {
	s.h.Distro.Arch = "Linux/Unix"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, ok := s.autoManager.(*ec2Manager)
	s.True(ok)
	provider, err := manager.getProvider(ctx, s.h, ec2Settings)
	s.NoError(err)
	s.Equal(spotProvider, provider)
	// subnet should be set based on vpc name
	s.Equal("subnet-654321", ec2Settings.SubnetId)
	s.Equal(s.h.Distro.Provider, evergreen.ProviderNameEc2Spot)

	s.h.UserHost = true
	provider, err = manager.getProvider(ctx, s.h, ec2Settings)
	s.NoError(err)
	s.Equal(onDemandProvider, provider)
}

func (s *EC2Suite) TestPersistInstanceId() {
	s.h.Id = "instance_id"
	s.h.Distro.Provider = evergreen.ProviderNameEc2Spot
	s.Require().NoError(s.h.Insert())
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	instanceID, err := manager.client.GetSpotInstanceId(context.Background(), s.h)
	s.Equal("instance_id", instanceID)
	s.NoError(err)
	s.Equal("instance_id", s.h.ExternalIdentifier)
}

func (s *EC2Suite) TestGetInstanceStatuses() {
	hosts := []host.Host{
		{
			Id: "sir-1",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2Spot,
			},
		},
		{
			Id: "i-2",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2OnDemand,
			},
		},
		{
			Id: "sir-3",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2Spot,
			},
		},
		{
			Id: "i-4",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2OnDemand,
			},
		},
		{
			Id: "i-5",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2OnDemand,
			},
		},
		{
			Id: "i-6",
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameEc2OnDemand,
			},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)

	// spot IDs returned doesn't match spot IDs submitted
	mock.DescribeSpotInstanceRequestsOutput = &ec2.DescribeSpotInstanceRequestsOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			&ec2.SpotInstanceRequest{
				InstanceId:            aws.String("sir-1"),
				State:                 aws.String(SpotStatusActive),
				SpotInstanceRequestId: aws.String("1"),
			},
		},
	}
	s.True(ok)
	batchManager, ok := s.onDemandManager.(BatchManager)
	s.True(ok)
	s.NotNil(batchManager)
	_, err := batchManager.GetInstanceStatuses(ctx, hosts)
	s.Error(err, "return an error if the number of spot IDs returned is different from submitted")

	mock.DescribeSpotInstanceRequestsOutput = &ec2.DescribeSpotInstanceRequestsOutput{
		SpotInstanceRequests: []*ec2.SpotInstanceRequest{
			// This host returns with no id
			&ec2.SpotInstanceRequest{
				InstanceId:            aws.String("i-3"),
				State:                 aws.String(SpotStatusActive),
				SpotInstanceRequestId: aws.String("sir-3"),
			},
			&ec2.SpotInstanceRequest{
				SpotInstanceRequestId: aws.String("sir-1"),
				State:                 aws.String(ec2.SpotInstanceStateFailed),
			},
		},
	}
	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-3"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName: aws.String("public_dns_name_2"),
						Placement: &ec2.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
							&ec2.InstanceBlockDeviceMapping{
								Ebs: &ec2.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id"),
								},
							},
						},
					},
				},
			},
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-2"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName: aws.String("public_dns_name_1"),
						Placement: &ec2.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
							&ec2.InstanceBlockDeviceMapping{
								Ebs: &ec2.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id"),
								},
							},
						},
					},
				},
			},
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-6"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameShuttingDown),
						},
					},
				},
			},
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-4"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName: aws.String("public_dns_name_3"),
						Placement: &ec2.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []*ec2.InstanceBlockDeviceMapping{
							&ec2.InstanceBlockDeviceMapping{
								Ebs: &ec2.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id"),
								},
							},
						},
					},
				},
			},
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-5"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameTerminated),
						},
					},
				},
			},
		},
	}
	statuses, err := batchManager.GetInstanceStatuses(ctx, hosts)
	s.NoError(err)
	s.Len(mock.DescribeSpotInstanceRequestsInput.SpotInstanceRequestIds, 2)
	s.Len(mock.DescribeInstancesInput.InstanceIds, 5)
	s.Equal("i-2", *mock.DescribeInstancesInput.InstanceIds[0])
	s.Equal("i-4", *mock.DescribeInstancesInput.InstanceIds[1])
	s.Equal("i-5", *mock.DescribeInstancesInput.InstanceIds[2])
	s.Equal("i-6", *mock.DescribeInstancesInput.InstanceIds[3])
	s.Equal("i-3", *mock.DescribeInstancesInput.InstanceIds[4])
	s.Len(statuses, 6)
	s.Equal(statuses, []CloudStatus{
		StatusFailed,
		StatusRunning,
		StatusRunning,
		StatusRunning,
		StatusTerminated,
		StatusTerminated,
	})

	s.Equal("public_dns_name_1", hosts[1].Host)
	s.Equal("public_dns_name_2", hosts[2].Host)
	s.Equal("public_dns_name_3", hosts[3].Host)

	s.Equal("i-3", hosts[2].ExternalIdentifier)
}

func (s *EC2Suite) TestGetRegion() {
	ec2Settings := &EC2ProviderSettings{}
	r := ec2Settings.getRegion()
	s.Equal(defaultRegion, r)

	(*s.h.Distro.ProviderSettings)["region"] = defaultRegion
	s.NoError(ec2Settings.fromDistroSettings(s.h.Distro))
	r = ec2Settings.getRegion()
	s.Equal(defaultRegion, r)

	(*s.h.Distro.ProviderSettings)["region"] = "us-west-2"
	s.NoError(ec2Settings.fromDistroSettings(s.h.Distro))
	r = ec2Settings.getRegion()
	s.Equal("us-west-2", r)
}

func (s *EC2Suite) TestUserDataExpand() {
	expanded, err := expandUserData("${test} a thing", s.autoManager.(*ec2Manager).settings.Expansions)
	s.NoError(err)
	s.Equal("expand a thing", expanded)
}

func (s *EC2Suite) TestGetSecurityGroup() {
	settings := EC2ProviderSettings{
		SecurityGroupIDs: []string{"sg-1"},
	}
	s.Equal([]*string{aws.String("sg-1")}, settings.getSecurityGroups())
	settings = EC2ProviderSettings{
		SecurityGroupIDs: []string{"sg-1"},
	}
	s.Equal([]*string{aws.String("sg-1")}, settings.getSecurityGroups())
	settings = EC2ProviderSettings{
		SecurityGroupIDs: []string{"sg-1", "sg-2"},
	}
	s.Equal([]*string{aws.String("sg-1"), aws.String("sg-2")}, settings.getSecurityGroups())
}

func (s *EC2Suite) TestCacheHostData() {
	ec2m := s.onDemandManager.(*ec2Manager)

	h := &host.Host{
		Id: "h1",
	}
	s.Require().NoError(h.Insert())

	instance := &ec2.Instance{Placement: &ec2.Placement{}}
	instance.Placement.AvailabilityZone = aws.String("us-east-1a")
	instance.LaunchTime = aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	instance.NetworkInterfaces = []*ec2.InstanceNetworkInterface{
		&ec2.InstanceNetworkInterface{
			Ipv6Addresses: []*ec2.InstanceIpv6Address{
				{
					Ipv6Address: aws.String("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
				},
			},
		},
	}
	instance.BlockDeviceMappings = []*ec2.InstanceBlockDeviceMapping{
		&ec2.InstanceBlockDeviceMapping{
			Ebs: &ec2.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
		},
	}
	instance.PublicDnsName = aws.String("public_dns_name")

	s.NoError(cacheHostData(context.Background(), h, instance, ec2m.client))

	s.Equal(*instance.Placement.AvailabilityZone, h.Zone)
	s.True(instance.LaunchTime.Equal(h.StartTime))
	s.Equal("2001:0db8:85a3:0000:0000:8a2e:0370:7334", h.IP)
	s.Equal([]string{"volume_id"}, h.VolumeIDs)
	s.Equal(int64(10), h.VolumeTotalSize)

	h, err := host.FindOneId("h1")
	s.Require().NoError(err)
	s.Require().NotNil(h)
	s.Equal(*instance.Placement.AvailabilityZone, h.Zone)
	s.True(instance.LaunchTime.Equal(h.StartTime))
	s.Equal("2001:0db8:85a3:0000:0000:8a2e:0370:7334", h.IP)
	s.Equal([]string{"volume_id"}, h.VolumeIDs)
	s.Equal(int64(10), h.VolumeTotalSize)
}

func (s *EC2Suite) TestFromDistroSettings() {
	d := distro.Distro{
		ProviderSettings: &map[string]interface{}{
			"key_name":           "key",
			"aws_access_key_id":  "key_id",
			"ami":                "ami",
			"instance_type":      "instance",
			"security_group_ids": []string{"abcdef"},
			"bid_price":          float64(0.001),
		},
	}

	ec2Settings := &EC2ProviderSettings{}
	s.NoError(ec2Settings.fromDistroSettings(d))
	s.Equal("key", ec2Settings.KeyName)
	s.Equal("ami", ec2Settings.AMI)
	s.Equal("instance", ec2Settings.InstanceType)
	s.Len(ec2Settings.SecurityGroupIDs, 1)
	s.Equal("abcdef", ec2Settings.SecurityGroupIDs[0])
	s.Equal(float64(0.001), ec2Settings.BidPrice)
}
