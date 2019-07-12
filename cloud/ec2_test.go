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
	h := &host.Host{}
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{"Linux", "instanceType", "US East (N. Virginia)"}: .1,
	}
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
	s.Equal("sg-123456", *runInput.SecurityGroups[0])
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SubnetId)
	describeInput := *mock.DescribeInstancesInput
	s.Equal("instance_id", *describeInput.InstanceIds[0])
	tagsInput := *mock.CreateTagsInput
	s.Equal("instance_id", *tagsInput.Resources[0])
	s.Len(tagsInput.Tags, 8)
	s.Equal(.1, h.ComputeCostPerHour)
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
	s.Equal(base64OfSomeUserData, *runInput.UserData)
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
	s.Equal(base64OfSomeUserData, *runInput.UserData)
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
	s.Equal(base64OfSomeUserData, *requestInput.LaunchSpecification.UserData)
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
	s.Equal(base64OfSomeUserData, *requestInput.LaunchSpecification.UserData)
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
	s.Equal("evg_auto_example_project", *runInput.KeyName)
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
	s.Equal(base64OfSomeUserData, *runInput.UserData)

	deleteInput := *mock.DeleteKeyPairInput
	s.Equal("evg_auto_"+project, *deleteInput.KeyName)
	createInput := *mock.CreateKeyPairInput
	s.Equal("evg_auto_"+project, *createInput.KeyName)

	k, err := model.GetAWSKeyForProject(project)
	s.NoError(err)
	s.Equal("evg_auto_"+project, k.Name)
	s.Equal("key_material", k.Value)

	// if we do it again, we shouldn't hit AWS for the key again
	deleteInput.KeyName = aws.String("")
	createInput.KeyName = aws.String("")
	_, err = s.onDemandManager.SpawnHost(ctx, h)
	s.NoError(err)
	s.Equal("", *deleteInput.KeyName)
	s.Equal("", *createInput.KeyName)
	k, err = model.GetAWSKeyForProject(project)
	s.NoError(err)
	s.Equal("evg_auto_"+project, k.Name)
	s.Equal("key_material", k.Value)
}
func (s *EC2Suite) TestGetInstanceStatus() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &host.Host{}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	status, err := s.onDemandManager.GetInstanceStatus(ctx, h)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	status, err = s.onDemandManager.GetInstanceStatus(ctx, h)
	s.NoError(err)
	s.Equal(StatusRunning, status)
}

func (s *EC2Suite) TestTerminateInstance() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &host.Host{Id: "host_id"}
	s.NoError(h.Insert())
	s.NoError(s.onDemandManager.TerminateInstance(ctx, h, evergreen.User))
	found, err := host.FindOne(host.ById("host_id"))
	s.Equal(evergreen.HostTerminated, found.Status)
	s.NoError(err)
}

func (s *EC2Suite) TestIsUp() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h := &host.Host{
		Distro: distro.Distro{},
	}
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	up, err := s.onDemandManager.IsUp(ctx, h)
	s.True(up)
	s.NoError(err)

	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	up, err = s.onDemandManager.IsUp(ctx, h)
	s.True(up)
	s.NoError(err)
}

func (s *EC2Suite) TestOnUp() {
	s.NoError(s.onDemandManager.OnUp(context.Background(), &host.Host{}))
}

func (s *EC2Suite) TestGetDNSName() {
	h := host.Host{Id: "instance_id"}
	s.Require().NoError(h.Insert())
	dns, err := s.onDemandManager.GetDNSName(context.Background(), &h)
	s.Equal("public_dns_name", dns)
	s.NoError(err)

	s.Equal(h.IP, MockIPV6)
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
	d := distro.Distro{Id: "foo"}
	id := d.GenerateName()
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, ok := s.autoManager.(*ec2Manager)
	s.True(ok)
	provider, err := manager.getProvider(ctx, h, ec2Settings)
	s.NoError(err)
	s.Equal(spotProvider, provider)
	// subnet should be set based on vpc name
	s.Equal("subnet-654321", ec2Settings.SubnetId)
	s.Equal(h.Distro.Provider, evergreen.ProviderNameEc2Spot)

	h.UserHost = true
	provider, err = manager.getProvider(ctx, h, ec2Settings)
	s.NoError(err)
	s.Equal(onDemandProvider, provider)
}

func (s *EC2Suite) TestPersistInstanceId() {
	h := &host.Host{Id: "instance_id"}
	s.Require().NoError(h.Insert())
	_, err := s.onDemandManager.GetDNSName(context.Background(), h)
	s.NoError(err)
	s.Equal("instance_id", h.ExternalIdentifier)
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
	s.Equal(*mock.DescribeInstancesInput.InstanceIds[0], "i-3")
	s.Equal(*mock.DescribeInstancesInput.InstanceIds[1], "i-2")
	s.Equal(*mock.DescribeInstancesInput.InstanceIds[2], "i-4")
	s.Equal(*mock.DescribeInstancesInput.InstanceIds[3], "i-5")
	s.Equal(*mock.DescribeInstancesInput.InstanceIds[4], "i-6")
	s.Len(statuses, 6)
	s.Equal(statuses, []CloudStatus{
		StatusFailed,
		StatusRunning,
		StatusRunning,
		StatusRunning,
		StatusTerminated,
		StatusTerminated,
	})
}

func (s *EC2Suite) TestGetRegion() {
	h := &host.Host{
		Distro: distro.Distro{
			ProviderSettings: &map[string]interface{}{},
		},
	}
	r, err := getRegion(h)
	s.NoError(err)
	s.Equal(defaultRegion, r)

	h = &host.Host{
		Distro: distro.Distro{
			ProviderSettings: &map[string]interface{}{
				"region": defaultRegion,
			},
		},
	}
	r, err = getRegion(h)
	s.NoError(err)
	s.Equal(defaultRegion, r)

	h = &host.Host{
		Distro: distro.Distro{
			ProviderSettings: &map[string]interface{}{
				"region": "us-west-2",
			},
		},
	}
	r, err = getRegion(h)
	s.NoError(err)
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
