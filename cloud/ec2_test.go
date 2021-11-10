package cloud

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	someUserData         = "#!/bin/bash\necho foo"
	base64OfSomeUserData = base64.StdEncoding.EncodeToString([]byte(someUserData))
)

type EC2Suite struct {
	suite.Suite
	onDemandOpts              *EC2ManagerOptions
	onDemandManager           Manager
	onDemandWithRegionOpts    *EC2ManagerOptions
	onDemandWithRegionManager Manager
	spotOpts                  *EC2ManagerOptions
	spotManager               Manager
	autoOpts                  *EC2ManagerOptions
	autoManager               Manager
	impl                      *ec2Manager
	mock                      *awsClientMock
	h                         *host.Host
	distro                    distro.Distro
	volume                    *host.Volume

	env evergreen.Environment
	ctx context.Context
}

func TestEC2Suite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := &EC2Suite{
		env: testutil.NewEnvironment(ctx, t),
		ctx: ctx,
	}
	suite.Run(t, s)
}

func (s *EC2Suite) SetupTest() {
	s.Require().NoError(db.ClearCollections(host.Collection, host.VolumesCollection, task.Collection, model.ProjectVarsCollection))
	s.onDemandOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: onDemandProvider,
	}
	s.onDemandManager = &ec2Manager{env: s.env, EC2ManagerOptions: s.onDemandOpts}
	_ = s.onDemandManager.Configure(s.ctx, &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				DefaultSecurityGroup: "sg-default",
			},
		},
	})
	s.onDemandWithRegionOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: onDemandProvider,
		region:   "test-region",
	}
	s.onDemandWithRegionManager = &ec2Manager{env: s.env, EC2ManagerOptions: s.onDemandWithRegionOpts}
	_ = s.onDemandManager.Configure(s.ctx, &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	s.spotOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: spotProvider,
	}
	s.spotManager = &ec2Manager{env: s.env, EC2ManagerOptions: s.spotOpts}
	_ = s.spotManager.Configure(s.ctx, &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	s.autoOpts = &EC2ManagerOptions{
		client:   &awsClientMock{},
		provider: autoProvider,
	}
	s.autoManager = &ec2Manager{env: s.env, EC2ManagerOptions: s.autoOpts}
	_ = s.autoManager.Configure(s.ctx, &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
	})
	var ok bool
	s.impl, ok = s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)

	// Clear mock
	s.mock, ok = s.impl.client.(*awsClientMock)
	s.Require().True(ok)
	s.mock.Instance = nil

	s.distro = distro.Distro{
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("key_name", "key"),
			birch.EC.String("aws_access_key_id", "key_id"),
			birch.EC.String("ami", "ami"),
			birch.EC.String("instance_type", "instance"),
			birch.EC.Double("bid_price", 0.001),
			birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
		)},
		Provider: evergreen.ProviderNameEc2OnDemand,
	}

	s.h = &host.Host{
		Id:     "h1",
		Distro: s.distro,
		InstanceTags: []host.Tag{
			{
				Key:           "key-1",
				Value:         "val-1",
				CanBeModified: true,
			},
		},
	}

	s.volume = &host.Volume{
		ID:         "test-volume",
		CreatedBy:  "test-user",
		Type:       "standard",
		Size:       32,
		Expiration: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}
}

func (s *EC2Suite) TestConstructor() {
	s.Implements((*Manager)(nil), &ec2Manager{env: s.env, EC2ManagerOptions: s.onDemandOpts})
	s.Implements((*BatchManager)(nil), &ec2Manager{env: s.env, EC2ManagerOptions: s.onDemandOpts})
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
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	err := s.onDemandManager.Configure(ctx, settings)
	s.Error(err)

	// No region specified
	settings.Providers.AWS.EC2Keys = []evergreen.EC2Key{
		{Key: "default-key", Secret: "default-secret"},
	}
	err = s.onDemandManager.Configure(ctx, settings)
	s.NoError(err)
	ec2m, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	creds, err := ec2m.credentials.Get()
	s.NoError(err)
	s.Equal("default-key", creds.AccessKeyID)
	s.Equal("default-secret", creds.SecretAccessKey)

	// config missing key or secret
	settings.Providers.AWS.EC2Keys = []evergreen.EC2Key{
		{Key: "test-key", Secret: ""},
	}
	err = s.onDemandWithRegionManager.Configure(ctx, settings)
	s.Error(err)
}

func (s *EC2Suite) TestSpawnHostInvalidInput() {
	h := &host.Host{
		Distro: distro.Distro{
			Provider: "foo",
			Id:       "id",
		},
	}

	ctx, cancel := context.WithCancel(s.ctx)
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
	s.h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", "keyName"),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}
	s.Require().NoError(s.h.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	_, err := s.onDemandManager.SpawnHost(ctx, s.h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.Equal("instanceType", *runInput.InstanceType)
	s.Equal("keyName", *runInput.KeyName)
	s.Require().Len(runInput.BlockDeviceMappings, 1)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Equal("sg-123456", *runInput.SecurityGroups[0])
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}

func (s *EC2Suite) TestSpawnHostVPCOnDemand() {
	pkgCachingPriceFetcher.ec2Prices = map[odInfo]float64{
		odInfo{"Linux", "instanceType", "US East (N. Virginia)"}: .1,
	}
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", "keyName"),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.Boolean("is_vpc", true),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}
	s.Require().NoError(h.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
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
}

func (s *EC2Suite) TestSpawnHostClassicSpot() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", "keyName"),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}
	s.Require().NoError(h.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	_, err := s.spotManager.SpawnHost(ctx, h)
	s.NoError(err)

	manager, ok := s.spotManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	s.Require().NotNil(mock.RequestSpotInstancesInput)
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
}

func (s *EC2Suite) TestSpawnHostVPCSpot() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2Spot
	h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", "keyName"),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.Boolean("is_vpc", true),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}
	s.Require().NoError(h.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	_, err := s.spotManager.SpawnHost(ctx, h)
	s.NoError(err)

	manager, ok := s.spotManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RequestSpotInstancesInput)
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
}

func (s *EC2Suite) TestNoKeyAndNotSpawnHostForTaskShouldFail() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", ""),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.Boolean("is_vpc", true),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}
	s.Require().NoError(h.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
	s.Error(err)
}

func (s *EC2Suite) TestSpawnHostForTask() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	h.Distro.ProviderSettingsList = []*birch.Document{birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("key_name", ""),
		birch.EC.String("subnet_id", "subnet-123456"),
		birch.EC.String("user_data", someUserData),
		birch.EC.Boolean("is_vpc", true),
		birch.EC.String("region", evergreen.DefaultEC2Region),
		birch.EC.SliceString("security_group_ids", []string{"sg-123456"}),
		birch.EC.Array("mount_points", birch.NewArray(
			birch.VC.Document(birch.NewDocument(
				birch.EC.String("device_name", "device"),
				birch.EC.String("virtual_name", "virtual"),
			)),
		)),
	)}

	project := "example_project"
	t := &task.Task{
		Id:      "task_1",
		Project: project,
	}
	h.SpawnOptions.TaskID = "task_1"
	h.StartedBy = "task_1"
	h.SpawnOptions.SpawnedByTask = true
	s.Require().NoError(h.Insert())
	s.Require().NoError(t.Insert())
	newVars := &model.ProjectVars{
		Id: project,
		Vars: map[string]string{
			model.ProjectAWSSSHKeyName:  "evg_auto_example_project",
			model.ProjectAWSSSHKeyValue: "key_material",
		},
	}
	s.Require().NoError(newVars.Insert())

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	_, err := s.onDemandManager.SpawnHost(ctx, h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
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

func (s *EC2Suite) TestModifyHost() {
	changes := host.HostModifyOptions{
		AddInstanceTags: []host.Tag{
			host.Tag{
				Key:           "key-2",
				Value:         "val-2",
				CanBeModified: true,
			},
		},
		DeleteInstanceTags: []string{"key-1"},
		InstanceType:       "instance-type-2",
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.h.Status = evergreen.HostRunning
	s.Require().NoError(s.h.Insert())
	s.Error(s.onDemandManager.ModifyHost(ctx, s.h, changes))
	s.Require().NoError(s.h.Remove())

	s.h.Status = evergreen.HostStopped
	s.Require().NoError(s.h.Insert())
	s.NoError(s.onDemandManager.ModifyHost(ctx, s.h, changes))
	found, err := host.FindOne(host.ById(s.h.Id))
	s.NoError(err)
	s.Equal([]host.Tag{host.Tag{Key: "key-2", Value: "val-2", CanBeModified: true}}, found.InstanceTags)
	s.Equal(changes.InstanceType, found.InstanceType)

	intent := host.Host{
		Id:           "evg-1234",
		NoExpiration: false,
	}
	s.NoError(intent.Insert())
	noExpiration := true
	changes = host.HostModifyOptions{NoExpiration: &noExpiration}
	s.NoError(s.onDemandManager.ModifyHost(ctx, s.h, changes))
	found, err = host.FindOne(host.ById(s.h.Id))
	s.NoError(err)
	s.True(found.NoExpiration)

	volumeToMount := host.Volume{
		ID:               "thang",
		AvailabilityZone: "us-east-1a",
	}
	s.Require().NoError(volumeToMount.Insert())
	s.h.Zone = "us-east-1a"
	s.Require().NoError(s.h.Remove())
	s.Require().NoError(s.h.Insert())
	changes = host.HostModifyOptions{
		AttachVolume: "thang",
	}
	s.NoError(s.onDemandManager.ModifyHost(ctx, s.h, changes))
	found, err = host.FindOne(host.ById(s.h.Id))
	s.NoError(err)
	s.Require().NoError(s.h.Remove())
}

func (s *EC2Suite) TestGetInstanceStatus() {
	ctx, cancel := context.WithCancel(s.ctx)
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
	s.Require().Len(s.h.Volumes, 1)
	s.Equal("volume_id", s.h.Volumes[0].VolumeID)

	s.h.Distro.Provider = evergreen.ProviderNameEc2Spot
	s.h.Id = "instance_id"
	status, err = s.onDemandManager.GetInstanceStatus(ctx, s.h)
	s.NoError(err)
	s.Equal(StatusRunning, status)

	s.Equal("instance_id", s.h.ExternalIdentifier)
}

func (s *EC2Suite) TestTerminateInstance() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.NoError(s.h.Insert())
	s.NoError(s.onDemandManager.TerminateInstance(ctx, s.h, evergreen.User, ""))
	found, err := host.FindOne(host.ById("h1"))
	s.Equal(evergreen.HostTerminated, found.Status)
	s.NoError(err)
}

func (s *EC2Suite) TestTerminateInstanceWithUserDataBootstrappedHost() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	s.Require().NoError(db.ClearCollections(host.Collection, user.Collection))
	defer func() {
		s.NoError(db.ClearCollections(host.Collection, user.Collection))
	}()

	s.h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
	s.NoError(s.h.Insert())

	creds, err := s.h.GenerateJasperCredentials(ctx, s.env)
	s.Require().NoError(err)
	s.Require().NoError(s.h.SaveJasperCredentials(ctx, s.env, creds))

	_, err = s.h.JasperCredentials(ctx, s.env)
	s.Require().NoError(err)

	s.NoError(s.onDemandManager.TerminateInstance(ctx, s.h, evergreen.User, ""))

	_, err = s.h.JasperCredentials(ctx, s.env)
	s.Error(err)
}

func (s *EC2Suite) TestStopInstance() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	hosts := []*host.Host{
		&host.Host{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
		&host.Host{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
		&host.Host{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
	}
	for _, h := range hosts {
		h.Distro = s.distro
		s.NoError(h.Insert())
	}

	s.Error(s.onDemandManager.StopInstance(ctx, hosts[0], evergreen.User))
	s.Error(s.onDemandManager.StopInstance(ctx, hosts[1], evergreen.User))
	s.NoError(s.onDemandManager.StopInstance(ctx, hosts[2], evergreen.User))
	found, err := host.FindOne(host.ById("host-running"))
	s.NoError(err)
	s.Equal(evergreen.HostStopped, found.Status)
}

func (s *EC2Suite) TestStartInstance() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	hosts := []*host.Host{
		&host.Host{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
		&host.Host{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
			Host:   "old_dns_name",
			IPv4:   "1.1.1.1",
		},
	}
	for _, h := range hosts {
		h.Distro = s.distro
		s.NoError(h.Insert())
	}

	s.Error(s.onDemandManager.StartInstance(ctx, hosts[0], evergreen.User))
	s.NoError(s.onDemandManager.StartInstance(ctx, hosts[1], evergreen.User))
	found, err := host.FindOne(host.ById("host-stopped"))
	s.NoError(err)
	s.Equal(evergreen.HostRunning, found.Status)
	s.Equal("public_dns_name", found.Host)
	s.Equal("12.34.56.78", found.IPv4)
}

func (s *EC2Suite) TestIsUp() {
	ctx, cancel := context.WithCancel(s.ctx)
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

func (s *EC2Suite) TestOnUpNoopsForOnDemandInstance() {
	s.Require().NoError(s.h.Insert())
	s.Require().NoError(s.spotManager.OnUp(s.ctx, s.h))
	manager, ok := s.spotManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)
	s.Nil(mock.DescribeVolumesInput)
	s.Zero(mock.CreateTagsInput)
}

func (s *EC2Suite) TestOnUpTagsForSpotInstance() {
	s.h.Distro.Provider = evergreen.ProviderNameEc2Spot
	s.h.ExternalIdentifier = "instance_id"
	s.NoError(s.h.Insert())

	s.Require().NoError(s.spotManager.OnUp(s.ctx, s.h))
	manager, ok := s.spotManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)
	s.Nil(mock.DescribeVolumesInput)

	s.Len(mock.CreateTagsInput.Resources, 2)
	s.Equal(s.h.ExternalIdentifier, *mock.CreateTagsInput.Resources[0])
	s.Equal("volume_id", *mock.CreateTagsInput.Resources[1])

	foundHost, err := host.FindOneId(s.h.Id)
	s.NoError(err)
	s.NotNil(foundHost)
	s.Require().Len(foundHost.Volumes, 1)
	s.Equal("volume_id", foundHost.Volumes[0].VolumeID)
	s.Equal("device_name", foundHost.Volumes[0].DeviceName)
}

func (s *EC2Suite) TestGetDNSName() {
	s.h.Host = "public_dns_name"
	dns, err := s.onDemandManager.GetDNSName(s.ctx, s.h)
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

	ctx, cancel := context.WithCancel(s.ctx)
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
	instanceID, err := manager.client.GetSpotInstanceId(s.ctx, s.h)
	s.Equal("instance_id", instanceID)
	s.NoError(err)
	s.Equal("instance_id", s.h.ExternalIdentifier)
}

func (s *EC2Suite) TestGetInstanceStatuses() {
	hosts := []host.Host{
		{
			Id: "sir-1",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2Spot,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
		{
			Id: "i-2",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
		{
			Id: "sir-3",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2Spot,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
		{
			Id: "i-4",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
		{
			Id: "i-5",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
		{
			Id: "i-6",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
		},
	}
	ctx, cancel := context.WithCancel(s.ctx)
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
						PublicDnsName:    aws.String("public_dns_name_2"),
						PrivateIpAddress: aws.String("2.2.2.2"),
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
						PublicDnsName:    aws.String("public_dns_name_1"),
						PrivateIpAddress: aws.String("1.1.1.1"),
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
						PublicDnsName:    aws.String("public_dns_name_3"),
						PrivateIpAddress: aws.String("3.3.3.3"),
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
	s.Equal("1.1.1.1", hosts[1].IPv4)
	s.Equal("public_dns_name_2", hosts[2].Host)
	s.Equal("2.2.2.2", hosts[2].IPv4)
	s.Equal("public_dns_name_3", hosts[3].Host)
	s.Equal("3.3.3.3", hosts[3].IPv4)

	s.Equal("i-3", hosts[2].ExternalIdentifier)
}

func (s *EC2Suite) TestGetInstanceStatusesTerminate() {
	hosts := []host.Host{
		{
			Id: "i-1",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
			Status: evergreen.HostStarting,
		},
		{
			Id: "i-2",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
				ProviderSettingsList: s.distro.ProviderSettingsList,
			},
			Status: evergreen.HostStarting,
		},
	}
	for _, h := range hosts {
		s.NoError(h.Insert())
	}

	manager := s.onDemandManager.(*ec2Manager)
	mock := manager.client.(*awsClientMock)
	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
		Reservations: []*ec2.Reservation{
			{
				Instances: []*ec2.Instance{
					{
						InstanceId: aws.String("i-1"),
						State: &ec2.InstanceState{
							Name: aws.String(ec2.InstanceStateNameRunning),
						},
						PublicDnsName:    aws.String("public_dns_name_2"),
						PrivateIpAddress: aws.String("2.2.2.2"),
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
		},
	}

	batchManager := s.onDemandManager.(BatchManager)
	s.NotNil(batchManager)
	_, err := batchManager.GetInstanceStatuses(context.Background(), hosts)
	s.Error(err)

	h, err := host.FindOneId("i-2")
	s.NoError(err)
	s.Equal(evergreen.HostTerminated, h.Status)
}

func (s *EC2Suite) TestGetRegion() {
	ec2Settings := &EC2ProviderSettings{}
	r := ec2Settings.getRegion()
	s.Equal(evergreen.DefaultEC2Region, r)

	s.h.Distro.ProviderSettingsList[0].Set(birch.EC.String("region", r))
	s.NoError(ec2Settings.FromDistroSettings(s.h.Distro, evergreen.DefaultEC2Region))
	r = ec2Settings.getRegion()
	s.Equal(evergreen.DefaultEC2Region, r)
}

func (s *EC2Suite) TestUserDataExpand() {
	expanded, err := expandUserData("${test} a thing", s.autoManager.(*ec2Manager).settings.Expansions)
	s.NoError(err)
	s.Equal("expand a thing", expanded)
}

func (s *EC2Suite) TestGetSecurityGroups() {
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

	s.Require().NoError(s.h.Insert())

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
			DeviceName: aws.String("device_name"),
			Ebs: &ec2.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
		},
	}
	instance.PublicDnsName = aws.String("public_dns_name")
	instance.PrivateIpAddress = aws.String("12.34.56.78")

	s.NoError(cacheHostData(s.ctx, s.h, instance, ec2m.client))

	s.Equal(*instance.Placement.AvailabilityZone, s.h.Zone)
	s.True(instance.LaunchTime.Equal(s.h.StartTime))
	s.Equal("2001:0db8:85a3:0000:0000:8a2e:0370:7334", s.h.IP)
	s.Equal([]host.VolumeAttachment{
		{
			VolumeID:   "volume_id",
			DeviceName: "device_name",
		},
	}, s.h.Volumes)

	h, err := host.FindOneId("h1")
	s.Require().NoError(err)
	s.Require().NotNil(h)
	s.Equal(*instance.Placement.AvailabilityZone, h.Zone)
	s.True(instance.LaunchTime.Equal(h.StartTime))
	s.Equal("2001:0db8:85a3:0000:0000:8a2e:0370:7334", h.IP)
	s.Equal("12.34.56.78", h.IPv4)
	s.Equal([]host.VolumeAttachment{
		{
			VolumeID:   "volume_id",
			DeviceName: "device_name",
		},
	}, h.Volumes)
}

func (s *EC2Suite) TestFromDistroSettings() {
	d := distro.Distro{
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("ami", "ami"),
			birch.EC.String("instance_type", "instance"),
			birch.EC.String("key_name", "key"),
			birch.EC.String("aws_access_key_id", "key_id"),
			birch.EC.String("subnet_id", "subnet-123456"),
			birch.EC.Double("bid_price", 0.001),
			birch.EC.String("region", evergreen.DefaultEC2Region),
			birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
		)},
	}

	ec2Settings := &EC2ProviderSettings{}
	s.NoError(ec2Settings.FromDistroSettings(d, evergreen.DefaultEC2Region))
	s.Equal("key", ec2Settings.KeyName)
	s.Equal("ami", ec2Settings.AMI)
	s.Equal("instance", ec2Settings.InstanceType)
	s.Require().Len(ec2Settings.SecurityGroupIDs, 1)
	s.Equal("abcdef", ec2Settings.SecurityGroupIDs[0])
	s.Equal(float64(0.001), ec2Settings.BidPrice)
	s.Equal(evergreen.DefaultEC2Region, ec2Settings.Region)

	// create provider list, choose by region
	settings2 := EC2ProviderSettings{
		Region:           "us-east-2",
		AMI:              "other_ami",
		InstanceType:     "other_instance",
		SecurityGroupIDs: []string{"ghijkl"},
		BidPrice:         float64(0.002),
		AWSKeyID:         "other_key_id",
		KeyName:          "other_key",
	}
	bytes, err := bson.Marshal(ec2Settings)
	s.NoError(err)
	doc1 := &birch.Document{}
	s.NoError(doc1.UnmarshalBSON(bytes))

	bytes, err = bson.Marshal(settings2)
	s.NoError(err)
	doc2 := &birch.Document{}
	s.NoError(doc2.UnmarshalBSON(bytes))
	d.ProviderSettingsList = []*birch.Document{doc1, doc2}

	s.NoError(ec2Settings.FromDistroSettings(d, "us-east-2"))
	s.Equal(ec2Settings.Region, "us-east-2")
	s.Equal(ec2Settings.InstanceType, "other_instance")
}

func (s *EC2Suite) TestGetEC2ManagerOptions() {
	d1 := distro.Distro{
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("region", evergreen.DefaultEC2Region),
			birch.EC.String("aws_access_key_id", "key"),
			birch.EC.String("aws_secret_access_key", "secret"),
		)},
	}

	managerOpts, err := GetManagerOptions(d1)
	s.NoError(err)
	s.Equal(evergreen.DefaultEC2Region, managerOpts.Region)
	s.Equal("key", managerOpts.ProviderKey)
	s.Equal("secret", managerOpts.ProviderSecret)
}

func (s *EC2Suite) TestGetEC2Key() {
	settings := &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{},
		},
	}
	key, secret, err := GetEC2Key(settings)
	s.Empty(key)
	s.Empty(secret)
	s.EqualError(err, "no EC2 keys in config")

	settings.Providers.AWS.EC2Keys = []evergreen.EC2Key{
		{Key: "test-key", Secret: "test-secret"},
	}
	key, secret, err = GetEC2Key(settings)
	s.Equal("test-key", key)
	s.Equal("test-secret", secret)
	s.NoError(err)
}

func (s *EC2Suite) TestSetNextSubnet() {
	s.Require().NoError(db.Clear(host.Collection))
	typeCache = map[instanceRegionPair][]evergreen.Subnet{
		{instanceType: "instance-type0", region: evergreen.DefaultEC2Region}: {
			{SubnetID: "sn0", AZ: evergreen.DefaultEC2Region + "a"},
			{SubnetID: "sn1", AZ: evergreen.DefaultEC2Region + "b"},
		},
		{instanceType: "instance-type1", region: evergreen.DefaultEC2Region}: {
			{SubnetID: "sn0", AZ: evergreen.DefaultEC2Region + "a"},
		},
	}

	h := &host.Host{
		Id:           "h0",
		InstanceType: "instance-type0",
		Distro: distro.Distro{
			ProviderSettingsList: []*birch.Document{
				birch.NewDocument(
					birch.EC.String("subnet_id", "not-supporting"),
					birch.EC.String("region", evergreen.DefaultEC2Region),
				),
			},
		},
	}
	s.Require().NoError(h.Insert())
	s.impl.region = evergreen.DefaultEC2Region

	// set to the first supporting subnet
	s.NoError(s.impl.setNextSubnet(context.Background(), h))
	s.Equal("sn0", h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValue())

	s.NoError(s.impl.setNextSubnet(context.Background(), h))
	s.Equal("sn1", h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValue())

	// wrap around
	s.NoError(s.impl.setNextSubnet(context.Background(), h))
	s.Equal("sn0", h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValue())

	// only supported by one subnet
	h.InstanceType = "instance-type1"
	s.Error(s.impl.setNextSubnet(context.Background(), h))
}

func (s *EC2Suite) TestCreateVolume() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	volume, err := s.onDemandManager.CreateVolume(ctx, s.volume)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.CreateVolumeInput
	s.Equal("standard", *input.VolumeType)

	foundVolume, err := host.FindVolumeByID(volume.ID)
	s.NotNil(foundVolume)
	s.NoError(err)
}

func (s *EC2Suite) TestDeleteVolume() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.volume.Insert())
	s.NoError(s.onDemandManager.DeleteVolume(ctx, s.volume))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.DeleteVolumeInput
	s.Equal("test-volume", *input.VolumeId)

	foundVolume, err := host.FindVolumeByID(s.volume.ID)
	s.Nil(foundVolume)
	s.NoError(err)
}

func (s *EC2Suite) TestAttachVolume() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Require().NoError(s.h.Insert())
	newAttachment := host.VolumeAttachment{
		VolumeID:   "test-volume",
		DeviceName: "test-device-name",
	}
	s.NoError(s.onDemandManager.AttachVolume(ctx, s.h, &newAttachment))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.AttachVolumeInput
	s.Equal("h1", *input.InstanceId)
	s.Equal("test-volume", *input.VolumeId)
	s.Equal("test-device-name", *input.Device)

	host, err := host.FindOneId(s.h.Id)
	s.NotNil(host)
	s.NoError(err)
	s.Contains(host.Volumes, newAttachment)
}

func (s *EC2Suite) TestAttachVolumeGenerateDeviceName() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Require().NoError(s.h.Insert())
	newAttachment := &host.VolumeAttachment{
		VolumeID: "test-volume",
	}

	s.NoError(s.onDemandManager.AttachVolume(ctx, s.h, newAttachment))

	s.Equal("test-volume", newAttachment.VolumeID)
	s.NotEqual("", newAttachment.DeviceName)
	s.Len(newAttachment.DeviceName, len("/dev/sdf"))
}

func (s *EC2Suite) TestDetachVolume() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	oldAttachment := host.VolumeAttachment{
		VolumeID:   s.volume.ID,
		DeviceName: "test-device-name",
	}
	s.h.Volumes = []host.VolumeAttachment{oldAttachment}
	s.Require().NoError(s.h.Insert())
	s.Require().NoError(s.volume.Insert())

	s.NoError(s.onDemandManager.DetachVolume(ctx, s.h, "test-volume"))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.DetachVolumeInput
	s.Equal("h1", *input.InstanceId)
	s.Equal("test-volume", *input.VolumeId)

	host, err := host.FindOneId(s.h.Id)
	s.NotNil(host)
	s.NoError(err)
	s.NotContains(host.Volumes, oldAttachment)
}

func (s *EC2Suite) TestModifyVolumeExpiration() {
	s.NoError(s.volume.Insert())
	newExpiration := s.volume.Expiration.Add(time.Hour)
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{Expiration: newExpiration}))

	vol, err := host.FindVolumeByID(s.volume.ID)
	s.NoError(err)
	s.True(newExpiration.Equal(vol.Expiration))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.CreateTagsInput
	s.Len(input.Tags, 1)
	s.Equal(newExpiration.Add(time.Hour*24*evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat), *input.Tags[0].Value)
}

func (s *EC2Suite) TestModifyVolumeNoExpiration() {
	s.NoError(s.volume.Insert())
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{NoExpiration: true}))

	vol, err := host.FindVolumeByID(s.volume.ID)
	s.NoError(err)
	s.True(time.Now().Add(evergreen.SpawnHostNoExpirationDuration).Sub(vol.Expiration) < time.Minute)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.CreateTagsInput
	s.Len(input.Tags, 1)
	s.Equal(vol.Expiration.Add(time.Hour*24*evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat), *input.Tags[0].Value)
}

func (s *EC2Suite) TestModifyVolumeSize() {
	s.NoError(s.volume.Insert())
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{Size: 100}))

	vol, err := host.FindVolumeByID(s.volume.ID)
	s.NoError(err)
	s.Equal(vol.Size, 100)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.True(ok)

	input := *mock.ModifyVolumeInput
	s.Equal(int(*input.Size), 100)
}

func (s *EC2Suite) TestModifyVolumeName() {
	s.NoError(s.volume.Insert())
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{NewName: "Some new thang"}))

	vol, err := host.FindVolumeByID(s.volume.ID)
	s.NoError(err)
	s.Equal(vol.DisplayName, "Some new thang")
}
