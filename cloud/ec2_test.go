package cloud

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restmodel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	someUserData         = "#!/bin/bash\necho foo"
	base64OfSomeUserData = base64.StdEncoding.EncodeToString([]byte(someUserData))
)

type EC2Suite struct {
	suite.Suite
	onDemandOpts    *EC2ManagerOptions
	onDemandManager Manager
	impl            *ec2Manager
	mock            *awsClientMock
	h               *host.Host
	distro          distro.Distro
	volume          *host.Volume

	env    evergreen.Environment
	ctx    context.Context
	cancel context.CancelFunc
}

func TestEC2Suite(t *testing.T) {
	s := &EC2Suite{}
	suite.Run(t, s)
}

func (s *EC2Suite) TearDownTest() {
	s.cancel()
}

func (s *EC2Suite) SetupTest() {
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	mockEnv := &mock.Environment{}
	s.Require().NoError(mockEnv.Configure(s.ctx))
	mockEnv.EvergreenSettings.Providers.AWS.PersistentDNS = evergreen.PersistentDNSConfig{
		HostedZoneID: "hosted_zone_id",
		Domain:       "example.com",
	}
	mockEnv.EvergreenSettings.SSH.TaskHostKey.Name = "keyName"
	s.env = mockEnv

	s.Require().NoError(db.ClearCollections(host.Collection, host.VolumesCollection, task.Collection, model.ProjectVarsCollection, fakeparameter.Collection, user.Collection))
	s.onDemandOpts = &EC2ManagerOptions{
		client: &awsClientMock{},
	}
	s.onDemandManager = &ec2Manager{env: s.env, EC2ManagerOptions: s.onDemandOpts}
	_ = s.onDemandManager.Configure(s.ctx, &evergreen.Settings{
		Expansions: map[string]string{"test": "expand"},
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				DefaultSecurityGroup: "sg-default",
			},
		},
		SSH: evergreen.SSHConfig{
			TaskHostKey: evergreen.SSHKeyPair{
				Name:      "task-host-key",
				SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/top-secret-private-key",
			},
			SpawnHostKey: evergreen.SSHKeyPair{
				Name:      "spawn-host-key",
				SecretARN: "arn:aws:secretsmanager:us-east-1:012345678901:secret/confidential-private-key",
			},
		},
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
	s.Empty(b)

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
		VolumeType: "gp3",
		DeviceName: "device",
		Size:       10,
		Iops:       100,
		Throughput: 150,
		SnapshotID: "snapshot-1",
	}
	b, err = makeBlockDeviceMappings([]MountPoint{ebsMount})
	s.NoError(err)
	s.Len(b, 1)
	s.Equal("device", *b[0].DeviceName)
	s.Equal(int32(10), *b[0].Ebs.VolumeSize)
	s.Equal(int32(100), *b[0].Ebs.Iops)
	s.Equal(int32(150), *b[0].Ebs.Throughput)
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
	s.Empty(b)

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
		VolumeType: "gp3",
		DeviceName: "device",
		Size:       10,
		Iops:       100,
		Throughput: 150,
		SnapshotID: "snapshot-1",
	}
	b, err = makeBlockDeviceMappingsTemplate([]MountPoint{ebsMount})
	s.NoError(err)
	s.Len(b, 1)
	s.Equal("device", *b[0].DeviceName)
	s.Equal(int32(10), *b[0].Ebs.VolumeSize)
	s.Equal(int32(100), *b[0].Ebs.Iops)
	s.Equal(int32(150), *b[0].Ebs.Throughput)
	s.Equal("snapshot-1", *b[0].Ebs.SnapshotId)
}

func (s *EC2Suite) TestConfigure() {
	settings := &evergreen.Settings{}

	// No region or account specified.
	// No region specified.
	s.Require().NoError(s.onDemandManager.Configure(s.ctx, settings))
	ec2m, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	s.Zero(ec2m.account)
	s.Zero(ec2m.role)
	s.Equal(evergreen.DefaultEC2Region, ec2m.region)

	// Region and account specified.
	const (
		account = "test-account"
		role    = "test-role"
	)
	onDemandWithRegionOpts := &EC2ManagerOptions{
		client:  &awsClientMock{},
		account: account,
		region:  "test-region",
	}
	settings.Providers.AWS.AccountRoles = []evergreen.AWSAccountRoleMapping{
		{
			Account: account,
			Role:    role,
		},
	}

	onDemandWithRegionManager := &ec2Manager{env: s.env, EC2ManagerOptions: onDemandWithRegionOpts}
	s.Require().NoError(onDemandWithRegionManager.Configure(s.ctx, settings))
	s.Equal(account, onDemandWithRegionManager.account)
	s.Equal(role, onDemandWithRegionManager.role)
	s.Equal(onDemandWithRegionOpts.region, onDemandWithRegionManager.region)
}

func (s *EC2Suite) TestSpawnHostInvalidInput() {
	h := &host.Host{
		Distro: distro.Distro{
			Provider: "foo",
			Id:       "id",
		},
	}

	spawned, err := s.onDemandManager.SpawnHost(s.ctx, h)
	s.Nil(spawned)
	s.Error(err)
	s.EqualError(err, "can't spawn EC2 instance for distro 'id': distro provider is 'foo'")
}

func validEC2ProviderSettings() *birch.Document {
	return birch.NewDocument(
		birch.EC.String("ami", "ami"),
		birch.EC.String("instance_type", "instanceType"),
		birch.EC.String("iam_instance_profile_arn", "my_profile"),
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
	)
}

func (s *EC2Suite) TestSpawnHostClassicOnDemand() {
	s.h.Distro.Id = "distro_id"
	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	s.h.Distro.ProviderSettingsList = []*birch.Document{validEC2ProviderSettings()}
	s.Require().NoError(s.h.Insert(s.ctx))

	_, err := s.onDemandManager.SpawnHost(s.ctx, s.h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.EqualValues("instanceType", runInput.InstanceType)
	s.Equal("task-host-key", *runInput.KeyName)
	s.Require().Len(runInput.BlockDeviceMappings, 1)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Equal("sg-123456", runInput.SecurityGroups[0])
	s.Equal("my_profile", *runInput.IamInstanceProfile.Arn)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}

func (s *EC2Suite) TestSpawnHostVPCOnDemand() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	providerSettings := validEC2ProviderSettings()
	providerSettings.Append(birch.EC.Boolean("is_vpc", true))
	h.Distro.ProviderSettingsList = []*birch.Document{providerSettings}
	s.Require().NoError(h.Insert(s.ctx))

	_, err := s.onDemandManager.SpawnHost(s.ctx, h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.EqualValues("instanceType", runInput.InstanceType)
	s.Equal("my_profile", *runInput.IamInstanceProfile.Arn)
	s.Equal("task-host-key", *runInput.KeyName)
	s.Require().Len(runInput.NetworkInterfaces, 1)
	s.True(aws.ToBool(runInput.NetworkInterfaces[0].AssociatePublicIpAddress))
	s.Require().Len(runInput.NetworkInterfaces[0].Groups, 1)
	s.Equal("sg-123456", runInput.NetworkInterfaces[0].Groups[0])
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SecurityGroups)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}

func (s *EC2Suite) TestSpawnHostForTask() {
	h := &host.Host{}
	h.Distro.Id = "distro_id"
	h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	providerSettings := validEC2ProviderSettings()
	providerSettings.Append(birch.EC.Boolean("is_vpc", true))
	h.Distro.ProviderSettingsList = []*birch.Document{providerSettings}

	project := "example_project"
	t := &task.Task{
		Id:      "task_1",
		Project: project,
	}
	h.SpawnOptions.TaskID = "task_1"
	h.StartedBy = "task_1"
	h.SpawnOptions.SpawnedByTask = true
	s.Require().NoError(h.Insert(s.ctx))
	s.Require().NoError(t.Insert(s.ctx))

	s.Require().NoError(db.Clear(model.ProjectRefCollection))
	defer func() {
		s.NoError(db.Clear(model.ProjectRefCollection))
	}()
	pRef := &model.ProjectRef{
		Id: project,
	}
	s.Require().NoError(pRef.Insert(s.ctx))

	newVars := &model.ProjectVars{
		Id: pRef.Id,
		Vars: map[string]string{
			model.ProjectAWSSSHKeyName:  "evg_auto_example_project",
			model.ProjectAWSSSHKeyValue: "key_material",
		},
	}
	s.Require().NoError(newVars.Insert(s.ctx))

	_, err := s.onDemandManager.SpawnHost(s.ctx, h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.EqualValues("instanceType", runInput.InstanceType)
	s.Equal("my_profile", *runInput.IamInstanceProfile.Arn)
	s.Equal("evg_auto_evergreen", *runInput.KeyName)
	s.Require().Len(runInput.NetworkInterfaces, 1)
	s.True(aws.ToBool(runInput.NetworkInterfaces[0].AssociatePublicIpAddress))
	s.Require().Len(runInput.NetworkInterfaces[0].Groups, 1)
	s.Equal("sg-123456", runInput.NetworkInterfaces[0].Groups[0])
	s.Equal("subnet-123456", aws.ToString(runInput.NetworkInterfaces[0].SubnetId))
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SecurityGroups)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}

func (s *EC2Suite) TestSpawnHostForTaskWithoutPublicIPv4Address() {
	s.h.Distro.Id = "distro_id"
	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	providerSettings := validEC2ProviderSettings()
	providerSettings.Append(birch.EC.Boolean("is_vpc", true))
	providerSettings.Append(birch.EC.Boolean("do_not_assign_public_ipv4_address", true))
	s.h.Distro.ProviderSettingsList = []*birch.Document{providerSettings}
	s.Require().NoError(s.h.Insert(s.ctx))

	_, err := s.onDemandManager.SpawnHost(s.ctx, s.h)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	s.Require().NotNil(mock.RunInstancesInput)
	runInput := *mock.RunInstancesInput
	s.Equal("ami", *runInput.ImageId)
	s.EqualValues("instanceType", runInput.InstanceType)
	s.Zero(runInput.KeyName)
	s.Require().Len(runInput.NetworkInterfaces, 1)
	s.False(aws.ToBool(runInput.NetworkInterfaces[0].AssociatePublicIpAddress))
	s.Require().Len(runInput.NetworkInterfaces[0].Groups, 1)
	s.Equal("sg-123456", runInput.NetworkInterfaces[0].Groups[0])
	s.Equal("subnet-123456", aws.ToString(runInput.NetworkInterfaces[0].SubnetId))
	s.Require().Len(runInput.BlockDeviceMappings, 1)
	s.Equal("virtual", *runInput.BlockDeviceMappings[0].VirtualName)
	s.Equal("device", *runInput.BlockDeviceMappings[0].DeviceName)
	s.Equal("my_profile", *runInput.IamInstanceProfile.Arn)
	s.Nil(runInput.SecurityGroupIds)
	s.Nil(runInput.SecurityGroups)
	s.Nil(runInput.SubnetId)
	s.Equal(base64OfSomeUserData, *runInput.UserData)
}

func (s *EC2Suite) TestModifyHost() {
	changes := host.HostModifyOptions{
		AddInstanceTags: []host.Tag{
			{
				Key:           "key-2",
				Value:         "val-2",
				CanBeModified: true,
			},
		},
		DeleteInstanceTags: []string{"key-1"},
		InstanceType:       "instance-type-2",
	}

	s.h.Status = evergreen.HostRunning
	s.Require().NoError(s.h.Insert(s.ctx))
	s.Error(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	s.Require().NoError(s.h.Remove(s.ctx))

	s.h.CreationTime = time.Now()
	s.h.ExpirationTime = s.h.CreationTime.Add(time.Hour * 24 * 7)
	s.h.NoExpiration = false
	s.h.Status = evergreen.HostStopped
	s.Require().NoError(s.h.Insert(s.ctx))

	// updating instance tags and instance type
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	found, err := host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.Equal([]host.Tag{{Key: "key-2", Value: "val-2", CanBeModified: true}}, found.InstanceTags)
	s.Equal(changes.InstanceType, found.InstanceType)

	// updating host expiration
	prevExpirationTime := found.ExpirationTime
	changes = host.HostModifyOptions{
		AddHours: time.Hour * 24,
	}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	found, err = host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.True(found.ExpirationTime.Equal(prevExpirationTime.Add(changes.AddHours)))

	// trying to update host expiration past 30 days should error
	changes = host.HostModifyOptions{
		AddHours: time.Hour * 24 * evergreen.SpawnHostExpireDays,
	}
	s.Error(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))

	// trying to update host expiration before now should error
	changes = host.HostModifyOptions{
		AddHours: time.Hour * -24 * 2 * evergreen.SpawnHostExpireDays,
	}
	s.Error(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))

	// modifying host to have no expiration
	changes = host.HostModifyOptions{NoExpiration: utility.TruePtr()}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	found, err = host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.Require().NotZero(found)
	s.True(found.NoExpiration)
	s.NotZero(found.PersistentDNSName, "persistent DNS name should be assigned once host is unexpirable")
	s.NotZero(found.PublicIPv4)

	// reverting a host back to having an expiration
	changes = host.HostModifyOptions{NoExpiration: utility.FalsePtr()}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	found, err = host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.Require().NotZero(found)
	s.False(found.NoExpiration)
	s.Zero(found.PersistentDNSName, "persistent DNS name should be removed once host is expirable")
	s.Zero(found.PublicIPv4)

	// modifying host to have no expiration when it's currently stopped
	s.NoError(s.h.SetStatus(s.ctx, evergreen.HostStopped, "user", ""))
	changes = host.HostModifyOptions{NoExpiration: utility.TruePtr()}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	found, err = host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.Require().NotZero(found)
	s.True(found.NoExpiration)
	s.NotZero(found.PersistentDNSName, "persistent DNS name should not be assigned to stopped host")
	s.NotZero(found.PublicIPv4)

	// attaching a volume to host
	volumeToMount := host.Volume{
		ID:               "thang",
		AvailabilityZone: "us-east-1a",
	}
	s.Require().NoError(volumeToMount.Insert(s.ctx))
	s.h.Zone = "us-east-1a"
	s.Require().NoError(s.h.Remove(s.ctx))
	s.Require().NoError(s.h.Insert(s.ctx))
	changes = host.HostModifyOptions{
		AttachVolume: "thang",
	}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))
	_, err = host.FindOne(s.ctx, host.ById(s.h.Id))
	s.NoError(err)
	s.Require().NoError(s.h.Remove(s.ctx))
}

func (s *EC2Suite) TestModifyHostWithNewTemporaryExemption() {
	s.h.Status = evergreen.HostRunning
	s.Require().NoError(s.h.Insert(s.ctx))
	const hours = 5
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, host.HostModifyOptions{AddTemporaryExemptionHours: hours}))

	dbHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbHost)
	s.WithinDuration(time.Now().Add(hours*time.Hour), dbHost.SleepSchedule.TemporarilyExemptUntil, time.Minute, "should create new temporary exemption")
}

func (s *EC2Suite) TestModifyHostWithExistingTemporaryExemption() {
	s.h.Status = evergreen.HostRunning
	s.h.SleepSchedule.TemporarilyExemptUntil = utility.BSONTime(time.Now().Add(time.Hour))
	const hours = 5
	extendedExemption := s.h.SleepSchedule.TemporarilyExemptUntil.Add(hours * time.Hour)
	s.Require().NoError(s.h.Insert(s.ctx))
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, host.HostModifyOptions{AddTemporaryExemptionHours: hours}))

	dbHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbHost)
	s.True(extendedExemption.Equal(dbHost.SleepSchedule.TemporarilyExemptUntil), "should extend existing temporary exemption")
}

func (s *EC2Suite) TestModifyHostWithExpiredTemporaryExemption() {
	s.h.Status = evergreen.HostRunning
	s.h.SleepSchedule.TemporarilyExemptUntil = utility.BSONTime(time.Now().Add(-time.Hour))
	s.Require().NoError(s.h.Insert(s.ctx))
	const hours = 5
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, host.HostModifyOptions{AddTemporaryExemptionHours: hours}))

	dbHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(dbHost)
	s.WithinDuration(time.Now().Add(hours*time.Hour), dbHost.SleepSchedule.TemporarilyExemptUntil, time.Minute, "should create new temporary exemption rather than extend the expired one")
}

func (s *EC2Suite) TestModifyHostWithNewSleepSchedule() {
	s.h.NoExpiration = true
	s.Require().NoError(s.h.Insert(s.ctx))

	changes := host.HostModifyOptions{
		SleepScheduleOptions: host.SleepScheduleOptions{
			WholeWeekdaysOff: []time.Weekday{time.Saturday},
			DailyStartTime:   "10:00",
			DailyStopTime:    "20:00",
			TimeZone:         "America/New_York",
		},
	}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))

	modifiedHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(modifiedHost)
	s.ElementsMatch(changes.SleepScheduleOptions.WholeWeekdaysOff, modifiedHost.SleepSchedule.WholeWeekdaysOff)
	s.Equal(changes.SleepScheduleOptions.DailyStartTime, modifiedHost.SleepSchedule.DailyStartTime)
	s.Equal(changes.SleepScheduleOptions.DailyStopTime, modifiedHost.SleepSchedule.DailyStopTime)
	s.Equal(changes.SleepScheduleOptions.TimeZone, modifiedHost.SleepSchedule.TimeZone)
	s.NotZero(modifiedHost.SleepSchedule.NextStartTime)
	s.NotZero(modifiedHost.SleepSchedule.NextStopTime)
}

func (s *EC2Suite) TestModifyHostUpdateExistingSleepSchedule() {
	s.h.NoExpiration = true
	temporarilyExemptUntil := utility.BSONTime(time.Now().Add(time.Hour))
	s.h.SleepSchedule = host.SleepScheduleInfo{
		WholeWeekdaysOff:       []time.Weekday{time.Sunday},
		DailyStartTime:         "09:00",
		DailyStopTime:          "19:00",
		TimeZone:               "Antarctica/Casey",
		TemporarilyExemptUntil: temporarilyExemptUntil,
	}
	s.Require().NoError(s.h.Insert(s.ctx))

	changes := host.HostModifyOptions{
		SleepScheduleOptions: host.SleepScheduleOptions{
			WholeWeekdaysOff: []time.Weekday{time.Saturday},
			DailyStartTime:   "10:00",
			DailyStopTime:    "20:00",
			TimeZone:         "America/New_York",
		},
	}
	s.NoError(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))

	modifiedHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(modifiedHost)
	s.ElementsMatch(changes.SleepScheduleOptions.WholeWeekdaysOff, modifiedHost.SleepSchedule.WholeWeekdaysOff, "should update whole weekdays off")
	s.Equal(changes.SleepScheduleOptions.DailyStartTime, modifiedHost.SleepSchedule.DailyStartTime, "should update daily start time")
	s.Equal(changes.SleepScheduleOptions.DailyStopTime, modifiedHost.SleepSchedule.DailyStopTime, "should update daily stop time")
	s.Equal(changes.SleepScheduleOptions.TimeZone, modifiedHost.SleepSchedule.TimeZone, "should update time zone")
	s.True(temporarilyExemptUntil.Equal(modifiedHost.SleepSchedule.TemporarilyExemptUntil), "should not change temporary exemption")
}

func (s *EC2Suite) TestModifyHostInvalidSchedule() {
	s.h.NoExpiration = true
	temporarilyExemptUntil := utility.BSONTime(time.Now().Add(time.Hour))
	originalSchedule := host.SleepScheduleInfo{
		WholeWeekdaysOff:       []time.Weekday{time.Sunday},
		DailyStartTime:         "09:00",
		DailyStopTime:          "19:00",
		TimeZone:               "Antarctica/Casey",
		TemporarilyExemptUntil: temporarilyExemptUntil,
	}
	s.h.SleepSchedule = originalSchedule
	s.Require().NoError(s.h.Insert(s.ctx))

	changes := host.HostModifyOptions{
		SleepScheduleOptions: host.SleepScheduleOptions{
			DailyStartTime: "10:00",
			DailyStopTime:  "10:00",
			TimeZone:       "America/New_York",
		},
	}
	s.Error(s.onDemandManager.ModifyHost(s.ctx, s.h, changes))

	modifiedHost, err := host.FindOneId(s.ctx, s.h.Id)
	s.Require().NoError(err)
	s.Require().NotZero(modifiedHost)
	s.ElementsMatch(originalSchedule.WholeWeekdaysOff, modifiedHost.SleepSchedule.WholeWeekdaysOff)
	s.Equal(originalSchedule.DailyStartTime, modifiedHost.SleepSchedule.DailyStartTime)
	s.Equal(originalSchedule.DailyStopTime, modifiedHost.SleepSchedule.DailyStopTime)
	s.Equal(originalSchedule.TimeZone, modifiedHost.SleepSchedule.TimeZone)
	s.True(temporarilyExemptUntil.Equal(modifiedHost.SleepSchedule.TemporarilyExemptUntil))
}

func (s *EC2Suite) TestGetInstanceInformation() {
	s.Require().NoError(s.h.Insert(s.ctx))

	s.h.Distro.Provider = evergreen.ProviderNameEc2OnDemand
	info, err := s.onDemandManager.GetInstanceState(s.ctx, s.h)
	s.NoError(err)
	s.Equal(StatusRunning, info.Status)

	// instance information is cached in the host
	s.Equal("us-east-1a", s.h.Zone)
	s.True(s.h.StartTime.Equal(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)))
	s.Equal("public_dns_name", s.h.Host)
	s.Require().Len(s.h.Volumes, 1)
	s.Equal("volume_id", s.h.Volumes[0].VolumeID)
}

func (s *EC2Suite) TestTerminateInstance() {
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.onDemandManager.TerminateInstance(s.ctx, s.h, evergreen.User, ""))
	found, err := host.FindOne(s.ctx, host.ById("h1"))
	s.Equal(evergreen.HostTerminated, found.Status)
	s.NoError(err)
}

func (s *EC2Suite) TestTerminateInstanceWithUserDataBootstrappedHost() {
	s.h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
	s.NoError(s.h.Insert(s.ctx))

	creds, err := s.h.GenerateJasperCredentials(s.ctx, s.env)
	s.Require().NoError(err)
	s.Require().NoError(s.h.SaveJasperCredentials(s.ctx, s.env, creds))

	_, err = s.h.JasperCredentials(s.ctx, s.env)
	s.Require().NoError(err)

	s.NoError(s.onDemandManager.TerminateInstance(s.ctx, s.h, evergreen.User, ""))

	_, err = s.h.JasperCredentials(s.ctx, s.env)
	s.Error(err)
}

func (s *EC2Suite) TestStopInstance() {
	unstoppableHosts := []*host.Host{
		{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
	}
	for _, h := range unstoppableHosts {
		h.Distro = s.distro
		s.Require().NoError(h.Insert(s.ctx))
	}
	for _, h := range unstoppableHosts {
		s.Error(s.onDemandManager.StopInstance(s.ctx, h, false, evergreen.User))
	}

	stoppableHosts := []*host.Host{
		{
			Id:     "host-stopping",
			Status: evergreen.HostStopping,
		},
		{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
	}
	for _, h := range stoppableHosts {
		h.Distro = s.distro
		s.Require().NoError(h.Insert(s.ctx))
	}

	for _, h := range stoppableHosts {
		s.NoError(s.onDemandManager.StopInstance(s.ctx, h, false, evergreen.User))
		found, err := host.FindOne(s.ctx, host.ById(h.Id))
		s.NoError(err)
		s.Equal(evergreen.HostStopped, found.Status)
	}
}

func (s *EC2Suite) TestStopInstanceAndShouldKeepOff() {
	stoppableHosts := []*host.Host{
		{
			Id:     "host-stopping",
			Status: evergreen.HostStopping,
		},
		{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
	}
	for _, h := range stoppableHosts {
		h.Distro = s.distro
		s.Require().NoError(h.Insert(s.ctx))
	}

	for _, h := range stoppableHosts {
		s.NoError(s.onDemandManager.StopInstance(s.ctx, h, true, evergreen.User))
		found, err := host.FindOne(s.ctx, host.ById(h.Id))
		s.NoError(err)
		s.Equal(evergreen.HostStopped, found.Status)
		s.True(found.SleepSchedule.ShouldKeepOff)
	}
}

func (s *EC2Suite) TestStartInstance() {
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)
	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{}

	unstartableHosts := []*host.Host{
		{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
	}
	for _, h := range unstartableHosts {
		h.Distro = s.distro
		s.Require().NoError(h.Insert(s.ctx))
	}
	for _, h := range unstartableHosts {
		s.Error(s.onDemandManager.StartInstance(s.ctx, h, evergreen.User))
	}

	startableHosts := []*host.Host{
		{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
		{
			Id:            "host-stopped",
			Status:        evergreen.HostStopped,
			Host:          "old_dns_name",
			IPv4:          "1.1.1.1",
			SleepSchedule: host.SleepScheduleInfo{ShouldKeepOff: true},
		},
	}
	for _, h := range startableHosts {
		h.Distro = s.distro
		s.NoError(h.Insert(s.ctx))
	}
	for _, h := range startableHosts {
		s.NoError(s.onDemandManager.StartInstance(s.ctx, h, evergreen.User))

		found, err := host.FindOne(s.ctx, host.ById(h.Id))
		s.NoError(err)
		s.Equal(evergreen.HostRunning, found.Status)
		s.Equal("public_dns_name", found.Host)
		s.Equal("12.34.56.78", found.IPv4)
		s.False(found.SleepSchedule.ShouldKeepOff)
	}
}

func (s *EC2Suite) TestRebootInstance() {
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)
	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{}

	unstartableHosts := []*host.Host{
		{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
		{
			Id:     "host-stopping",
			Status: evergreen.HostStopping,
		},
		{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
	}
	for _, h := range unstartableHosts {
		h.Distro = s.distro
		s.Require().NoError(h.Insert(s.ctx))
	}
	for _, h := range unstartableHosts {
		s.Error(s.onDemandManager.RebootInstance(s.ctx, h, evergreen.User))
	}

	startableHosts := []*host.Host{
		{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
	}
	for _, h := range startableHosts {
		h.Distro = s.distro
		s.NoError(h.Insert(s.ctx))
	}
	for _, h := range startableHosts {
		s.NoError(s.onDemandManager.RebootInstance(s.ctx, h, evergreen.User))

		found, err := host.FindOne(s.ctx, host.ById(h.Id))
		s.NoError(err)
		s.Equal(evergreen.HostRunning, found.Status)
		s.Equal("public_dns_name", found.Host)
		s.Equal("12.34.56.78", found.IPv4)
		s.False(found.SleepSchedule.ShouldKeepOff)
	}
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

func (s *EC2Suite) TestTimeTilNextPaymentSUSE() {
	now := time.Now()
	thirtyMinutesAgo := now.Add(-30 * time.Minute)
	h := &host.Host{
		Distro: distro.Distro{
			Id:   "suse15-large",
			Arch: "linux",
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

func (s *EC2Suite) TestGetInstanceStatuses() {
	hosts := []host.Host{
		{
			Id: "i-1",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
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
			Id: "i-3",
			Distro: distro.Distro{
				Provider:             evergreen.ProviderNameEc2OnDemand,
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
	}
	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-4"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameShuttingDown,
						},
					},
				},
			},
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-2"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name_3"),
						PublicIpAddress:  aws.String("127.0.0.1"),
						PrivateIpAddress: aws.String("3.3.3.3"),
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id"),
								},
							},
						},
					},
				},
			},
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-3"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameTerminated,
						},
					},
				},
			},
		},
	}
	s.True(ok)
	batchManager, ok := s.onDemandManager.(BatchManager)
	s.True(ok)
	s.NotNil(batchManager)
	statuses, err := batchManager.GetInstanceStatuses(s.ctx, hosts)
	s.NoError(err, "does not error if some of the instances do not exist")
	s.Equal(map[string]CloudStatus{
		"i-1": StatusNonExistent,
		"i-2": StatusRunning,
		"i-3": StatusTerminated,
		"i-4": StatusTerminated,
	}, statuses)

	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-3"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name_3"),
						PublicIpAddress:  aws.String("127.0.0.3"),
						PrivateIpAddress: aws.String("3.3.3.3"),
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1c"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id3"),
								},
								DeviceName: aws.String("/dev/sda3"),
							},
						},
						NetworkInterfaces: []types.InstanceNetworkInterface{
							{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::3")}}},
						},
					},
				},
			},
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-1"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name_1"),
						PublicIpAddress:  aws.String("127.0.0.1"),
						PrivateIpAddress: aws.String("1.1.1.1"),
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id1"),
								},
								DeviceName: aws.String("/dev/sda1"),
							},
						},
						NetworkInterfaces: []types.InstanceNetworkInterface{
							{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::1")}}},
						},
					},
				},
			},
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-4"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameShuttingDown,
						},
					},
				},
			},
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-2"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name_2"),
						PublicIpAddress:  aws.String("127.0.0.2"),
						PrivateIpAddress: aws.String("2.2.2.2"),
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1b"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
									VolumeId: aws.String("volume_id2"),
								},
								DeviceName: aws.String("/dev/sda2"),
							},
						},
						NetworkInterfaces: []types.InstanceNetworkInterface{
							{Ipv6Addresses: []types.InstanceIpv6Address{{Ipv6Address: aws.String("::2")}}},
						},
					},
				},
			},
		},
	}
	statuses, err = batchManager.GetInstanceStatuses(s.ctx, hosts)
	s.NoError(err)
	s.Len(mock.DescribeInstancesInput.InstanceIds, 4)
	s.Equal("i-1", mock.DescribeInstancesInput.InstanceIds[0])
	s.Equal("i-2", mock.DescribeInstancesInput.InstanceIds[1])
	s.Equal("i-3", mock.DescribeInstancesInput.InstanceIds[2])
	s.Equal("i-4", mock.DescribeInstancesInput.InstanceIds[3])
	s.Len(statuses, 4)
	s.Equal(map[string]CloudStatus{
		"i-1": StatusRunning,
		"i-2": StatusRunning,
		"i-3": StatusRunning,
		"i-4": StatusTerminated,
	}, statuses)

	s.Equal("public_dns_name_1", hosts[0].Host)
	s.Equal("127.0.0.1", hosts[0].PublicIPv4)
	s.Equal("1.1.1.1", hosts[0].IPv4)
	s.Equal("::1", hosts[0].IP)
	s.Equal("us-east-1a", hosts[0].Zone)
	s.Require().Len(hosts[0].Volumes, 1)
	s.Equal("volume_id1", hosts[0].Volumes[0].VolumeID)
	s.Equal("/dev/sda1", hosts[0].Volumes[0].DeviceName)

	s.Equal("public_dns_name_2", hosts[1].Host)
	s.Equal("127.0.0.2", hosts[1].PublicIPv4)
	s.Equal("2.2.2.2", hosts[1].IPv4)
	s.Equal("::2", hosts[1].IP)
	s.Equal("us-east-1b", hosts[1].Zone)
	s.Require().Len(hosts[1].Volumes, 1)
	s.Equal("volume_id2", hosts[1].Volumes[0].VolumeID)
	s.Equal("/dev/sda2", hosts[1].Volumes[0].DeviceName)

	s.Equal("public_dns_name_3", hosts[2].Host)
	s.Equal("127.0.0.3", hosts[2].PublicIPv4)
	s.Equal("::3", hosts[2].IP)
	s.Equal("3.3.3.3", hosts[2].IPv4)
	s.Equal("us-east-1c", hosts[2].Zone)
	s.Require().Len(hosts[2].Volumes, 1)
	s.Equal("volume_id3", hosts[2].Volumes[0].VolumeID)
	s.Equal("/dev/sda3", hosts[2].Volumes[0].DeviceName)
}

func (s *EC2Suite) TestGetInstanceStatusesForNonexistentInstances() {
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
		s.NoError(h.Insert(s.ctx))
	}

	manager := s.onDemandManager.(*ec2Manager)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)
	mock.DescribeInstancesOutput = &ec2.DescribeInstancesOutput{
		Reservations: []types.Reservation{
			{
				Instances: []types.Instance{
					{
						InstanceId: aws.String("i-1"),
						State: &types.InstanceState{
							Name: types.InstanceStateNameRunning,
						},
						PublicDnsName:    aws.String("public_dns_name_2"),
						PrivateIpAddress: aws.String("2.2.2.2"),
						Placement: &types.Placement{
							AvailabilityZone: aws.String("us-east-1a"),
						},
						LaunchTime: aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)),
						BlockDeviceMappings: []types.InstanceBlockDeviceMapping{
							{
								Ebs: &types.EbsInstanceBlockDevice{
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
	statuses, err := batchManager.GetInstanceStatuses(context.Background(), hosts)
	s.NoError(err)
	s.Len(statuses, len(hosts), "should have one status for the existing host and one for the nonexistent host")
	s.Equal(StatusRunning, statuses[hosts[0].Id])
	s.Equal(StatusNonExistent, statuses[hosts[1].Id])
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
	expanded, err := expandUserData("${test} a thing", s.onDemandManager.(*ec2Manager).settings.Expansions)
	s.NoError(err)
	s.Equal("expand a thing", expanded)
}

func (s *EC2Suite) TestCacheHostData() {
	ec2m := s.onDemandManager.(*ec2Manager)

	s.Require().NoError(s.h.Insert(s.ctx))

	instance := &types.Instance{Placement: &types.Placement{}}
	instance.Placement.AvailabilityZone = aws.String("us-east-1a")
	instance.LaunchTime = aws.Time(time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC))
	instance.NetworkInterfaces = []types.InstanceNetworkInterface{
		{
			Ipv6Addresses: []types.InstanceIpv6Address{
				{
					Ipv6Address: aws.String("2001:0db8:85a3:0000:0000:8a2e:0370:7334"),
				},
			},
		},
	}
	instance.BlockDeviceMappings = []types.InstanceBlockDeviceMapping{
		{
			DeviceName: aws.String("device_name"),
			Ebs: &types.EbsInstanceBlockDevice{
				VolumeId: aws.String("volume_id"),
			},
		},
	}
	instance.PublicDnsName = aws.String("public_dns_name")
	instance.PublicIpAddress = aws.String("127.0.0.1")
	instance.PrivateIpAddress = aws.String("12.34.56.78")

	pair := hostInstancePair{host: s.h, instance: instance}
	s.NoError(cacheAllHostData(s.ctx, s.env, ec2m.client, pair))

	s.Equal(*instance.Placement.AvailabilityZone, s.h.Zone)
	s.True(instance.LaunchTime.Equal(s.h.StartTime))
	s.Equal("2001:0db8:85a3:0000:0000:8a2e:0370:7334", s.h.IP)
	s.Equal("public_dns_name", s.h.Host)
	s.Equal("127.0.0.1", s.h.PublicIPv4)
	s.Equal("12.34.56.78", s.h.IPv4)
	s.Equal([]host.VolumeAttachment{
		{
			VolumeID:   "volume_id",
			DeviceName: "device_name",
		},
	}, s.h.Volumes)

	h, err := host.FindOneId(s.ctx, "h1")
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
			birch.EC.String("region", evergreen.DefaultEC2Region),
			birch.EC.String("iam_instance_profile_arn", "a_new_arn"),
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
	s.Equal(evergreen.DefaultEC2Region, ec2Settings.Region)
	s.Equal("a_new_arn", ec2Settings.IAMInstanceProfileARN)

	// create provider list, choose by region
	settings2 := EC2ProviderSettings{
		Region:                "us-east-2",
		AMI:                   "other_ami",
		InstanceType:          "other_instance",
		SecurityGroupIDs:      []string{"ghijkl"},
		IAMInstanceProfileARN: "a_beautiful_profile",
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
	s.Equal("us-east-2", ec2Settings.Region)
	s.Equal("other_instance", ec2Settings.InstanceType)
	s.Equal("a_beautiful_profile", ec2Settings.IAMInstanceProfileARN)
}

func (s *EC2Suite) TestGetEC2ManagerOptions() {
	d1 := distro.Distro{
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("region", evergreen.DefaultEC2Region),
			birch.EC.String("aws_access_key_id", "key"),
			birch.EC.String("aws_secret_access_key", "secret"),
		)},
		ProviderAccount: "account",
	}

	managerOpts, err := GetManagerOptions(d1)
	s.NoError(err)
	s.Equal(d1.ProviderAccount, managerOpts.Account)
	s.Equal(evergreen.DefaultEC2Region, managerOpts.Region)
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
	s.Require().NoError(h.Insert(s.ctx))
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
	volume, err := s.onDemandManager.CreateVolume(s.ctx, s.volume)
	s.NoError(err)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.CreateVolumeInput
	s.EqualValues("standard", input.VolumeType)

	foundVolume, err := host.FindVolumeByID(s.ctx, volume.ID)
	s.NotNil(foundVolume)
	s.NoError(err)
}

func (s *EC2Suite) TestDeleteVolume() {
	s.NoError(s.volume.Insert(s.ctx))
	s.NoError(s.onDemandManager.DeleteVolume(s.ctx, s.volume))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.DeleteVolumeInput
	s.Equal("test-volume", *input.VolumeId)

	foundVolume, err := host.FindVolumeByID(s.ctx, s.volume.ID)
	s.Nil(foundVolume)
	s.NoError(err)
}

func (s *EC2Suite) TestAttachVolume() {
	s.Require().NoError(s.h.Insert(s.ctx))
	newAttachment := host.VolumeAttachment{
		VolumeID:   "test-volume",
		DeviceName: "test-device-name",
	}
	s.NoError(s.onDemandManager.AttachVolume(s.ctx, s.h, &newAttachment))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.AttachVolumeInput
	s.Equal("h1", *input.InstanceId)
	s.Equal("test-volume", *input.VolumeId)
	s.Equal("test-device-name", *input.Device)

	host, err := host.FindOneId(s.ctx, s.h.Id)
	s.NotNil(host)
	s.NoError(err)
	s.Contains(host.Volumes, newAttachment)
}

func (s *EC2Suite) TestAttachVolumeGenerateDeviceName() {
	s.Require().NoError(s.h.Insert(s.ctx))
	newAttachment := &host.VolumeAttachment{
		VolumeID: "test-volume",
	}

	s.NoError(s.onDemandManager.AttachVolume(s.ctx, s.h, newAttachment))

	s.Equal("test-volume", newAttachment.VolumeID)
	s.NotEqual("", newAttachment.DeviceName)
	s.Len(newAttachment.DeviceName, len("/dev/sdf"))
}

func (s *EC2Suite) TestDetachVolume() {
	oldAttachment := host.VolumeAttachment{
		VolumeID:   s.volume.ID,
		DeviceName: "test-device-name",
	}
	s.h.Volumes = []host.VolumeAttachment{oldAttachment}
	s.Require().NoError(s.h.Insert(s.ctx))
	s.Require().NoError(s.volume.Insert(s.ctx))

	s.NoError(s.onDemandManager.DetachVolume(s.ctx, s.h, "test-volume"))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.DetachVolumeInput
	s.Equal("h1", *input.InstanceId)
	s.Equal("test-volume", *input.VolumeId)

	host, err := host.FindOneId(s.ctx, s.h.Id)
	s.NotNil(host)
	s.NoError(err)
	s.NotContains(host.Volumes, oldAttachment)
}

func (s *EC2Suite) TestModifyVolumeExpiration() {
	s.NoError(s.volume.Insert(s.ctx))
	newExpiration := s.volume.Expiration.Add(time.Hour)
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{Expiration: newExpiration}))

	vol, err := host.FindVolumeByID(s.ctx, s.volume.ID)
	s.NoError(err)
	s.True(newExpiration.Equal(vol.Expiration))

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.CreateTagsInput
	s.Len(input.Tags, 1)
	s.Equal(newExpiration.Add(time.Hour*24*evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat), *input.Tags[0].Value)
}

func (s *EC2Suite) TestModifyVolumeNoExpiration() {
	s.NoError(s.volume.Insert(s.ctx))
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{NoExpiration: true}))

	vol, err := host.FindVolumeByID(s.ctx, s.volume.ID)
	s.NoError(err)
	s.Less(time.Now().Add(evergreen.SpawnHostNoExpirationDuration).Sub(vol.Expiration), time.Minute)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.CreateTagsInput
	s.Len(input.Tags, 1)
	s.Equal(vol.Expiration.Add(time.Hour*24*evergreen.SpawnHostExpireDays).Format(evergreen.ExpireOnFormat), *input.Tags[0].Value)
}

func (s *EC2Suite) TestModifyVolumeSize() {
	s.NoError(s.volume.Insert(s.ctx))
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{Size: 100}))

	vol, err := host.FindVolumeByID(s.ctx, s.volume.ID)
	s.NoError(err)
	s.EqualValues(100, vol.Size)

	manager, ok := s.onDemandManager.(*ec2Manager)
	s.Require().True(ok)
	mock, ok := manager.client.(*awsClientMock)
	s.Require().True(ok)

	input := *mock.ModifyVolumeInput
	s.EqualValues(100, *input.Size)
}

func (s *EC2Suite) TestModifyVolumeName() {
	s.NoError(s.volume.Insert(s.ctx))
	s.NoError(s.onDemandManager.ModifyVolume(context.Background(), s.volume, &restmodel.VolumeModifyOptions{NewName: "Some new thang"}))

	vol, err := host.FindVolumeByID(s.ctx, s.volume.ID)
	s.NoError(err)
	s.Equal("Some new thang", vol.DisplayName)
}
