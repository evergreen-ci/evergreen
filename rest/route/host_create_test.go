package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeHost(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(err)

	handler := hostCreateHandler{}

	d := distro.Distro{
		Id:       "archlinux-test",
		Aliases:  []string{"archlinux-alias"},
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("ami", "ami-123456"),
			birch.EC.String("region", "us-east-1"),
			birch.EC.String("instance_type", "t1.micro"),
			birch.EC.String("subnet_id", "subnet-12345678"),
			birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
		)},
	}
	require.NoError(d.Insert(ctx))

	sampleTask := &task.Task{
		Id: "task-id",
	}
	require.NoError(sampleTask.Insert(t.Context()))

	// spawn an evergreen distro
	c := apimodels.CreateHost{
		Distro:              "archlinux-test",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	handler.taskID = "task-id"
	foundDistro, err := distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err := data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	require.NotNil(h)

	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	assert.Equal("task-id", h.SpawnOptions.TaskID)
	ec2Settings := &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Empty(ec2Settings.KeyName)
	assert.True(ec2Settings.IsVpc)

	// test roundtripping
	h, err = host.FindOneByIdOrTag(ctx, h.Id)
	assert.NoError(err)
	require.NotNil(h)
	ec2Settings2 := &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings2.AMI)
	assert.Empty(ec2Settings2.KeyName)
	assert.True(ec2Settings2.IsVpc)

	// scope to build
	require.NoError(db.ClearCollections(task.Collection))
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert(t.Context()))
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		NumHosts:            "1",
		Scope:               "build",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	handler.taskID = "task-id"
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	assert.NotNil(h)
	ec2Settings = &cloud.EC2ProviderSettings{}
	assert.NoError(ec2Settings.FromDistroSettings(h.Distro, ""))
	assert.Equal("build-id", h.SpawnOptions.BuildID)
	assert.Empty(ec2Settings.KeyName)
	assert.True(ec2Settings.IsVpc)

	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	// Using an alias should resolve to the actual distro
	c = apimodels.CreateHost{
		Distro:              "archlinux-alias",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	handler.taskID = "task-id"
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	require.NoError(err)
	require.NotNil(h)

	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	assert.Equal("task-id", h.SpawnOptions.TaskID)
	ec2Settings = &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Empty(ec2Settings.KeyName)
	assert.True(ec2Settings.IsVpc)

	// override some evergreen distro settings
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Subnet:              "subnet-123456",
		Tenancy:             evergreen.EC2TenancyDedicated,
	}
	handler.createHost = c
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	assert.NotNil(h)

	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	ec2Settings = &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.EC2TenancyDedicated, ec2Settings.Tenancy)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)
	assert.True(ec2Settings.IsVpc)

	ec2Settings2 = &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings2.AMI)
	assert.Equal("subnet-123456", ec2Settings2.SubnetId)
	assert.Equal(evergreen.EC2TenancyDedicated, ec2Settings2.Tenancy)
	assert.True(ec2Settings2.IsVpc)

	// bring your own ami
	c = apimodels.CreateHost{
		AMI:                 "ami-654321",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		InstanceType:        "t1.micro",
		Subnet:              "subnet-123456",
		SecurityGroups:      []string{"1234"},
		Tenancy:             evergreen.EC2TenancyDedicated,
	}
	handler.createHost = c
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	require.NoError(err)
	require.NotNil(h)

	assert.Equal("", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider, "provider should be set to ec2 in the absence of a distro")
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	ec2Settings2 = &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-654321", ec2Settings2.AMI)
	assert.Equal("subnet-123456", ec2Settings2.SubnetId)
	assert.Equal(evergreen.EC2TenancyDedicated, ec2Settings.Tenancy)
	assert.True(ec2Settings2.IsVpc)

	// with multiple regions
	require.Len(d.ProviderSettingsList, 1)
	doc2 := d.ProviderSettingsList[0].Copy().Set(birch.EC.String("region", "us-west-1")).Set(birch.EC.String("ami", "ami-987654"))
	d.ProviderSettingsList = append(d.ProviderSettingsList, doc2)
	require.NoError(d.ReplaceOne(ctx))
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)

	ec2Settings2 = &cloud.EC2ProviderSettings{}
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, "us-east-1"))
	assert.Equal("ami-123456", ec2Settings2.AMI)

	handler.createHost.Region = "us-west-1"
	foundDistro, err = distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err = data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	require.Len(h.Distro.ProviderSettingsList, 1)
	ec2Settings2 = &cloud.EC2ProviderSettings{}
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, "us-west-1"))
	assert.Equal("ami-987654", ec2Settings2.AMI)
}

func TestHostCreateHandler(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))

	handler := hostCreateHandler{}
	d := distro.Distro{
		Id:       "archlinux-test",
		Aliases:  []string{"archlinux-alias"},
		Provider: evergreen.ProviderNameEc2OnDemand,
		ProviderSettingsList: []*birch.Document{birch.NewDocument(
			birch.EC.String("ami", "ami-123456"),
			birch.EC.String("region", "us-east-1"),
			birch.EC.String("instance_type", "t1.micro"),
			birch.EC.String("subnet_id", "subnet-12345678"),
			birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
		)},
	}
	require.NoError(d.Insert(ctx))

	sampleTask := &task.Task{
		Id:        "task-id",
		Execution: 0,
	}
	require.NoError(sampleTask.Insert(t.Context()))

	c := apimodels.CreateHost{
		Distro:              "archlinux-test",
		NumHosts:            "3",
		Scope:               "task",
		Subnet:              "sub",
		AMI:                 "ami",
		InstanceType:        "instance-type",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	foundDistro, err := distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)

	handler.createHost = c
	handler.distro = *foundDistro
	handler.env = env
	handler.taskID = "task-id"

	hosts, err := host.FindHostsSpawnedByTask(ctx, sampleTask.Id, sampleTask.Execution, append(evergreen.IsRunningOrWillRunStatuses, evergreen.HostUninitialized))
	assert.NoError(err)
	assert.Empty(hosts)

	assert.Equal(http.StatusOK, handler.Run(ctx).Status())

	// Properly creates correct number of hosts.
	hosts, err = host.FindHostsSpawnedByTask(ctx, sampleTask.Id, sampleTask.Execution, createdOrCreatingHostStatuses)
	assert.NoError(err)
	assert.Len(hosts, 3)

	// Calling create host route multiple times should not create more hosts than number requested.
	assert.Equal(http.StatusOK, handler.Run(ctx).Status())
	assert.Equal(http.StatusOK, handler.Run(ctx).Status())

	hosts, err = host.FindHostsSpawnedByTask(ctx, sampleTask.Id, sampleTask.Execution, createdOrCreatingHostStatuses)
	assert.NoError(err)
	assert.Len(hosts, 3)

	// If a partial amount of hosts were initially created, retry should create the rest.
	handler.createHost.NumHosts = "5"
	assert.Equal(http.StatusOK, handler.Run(ctx).Status())

	hosts, err = host.FindHostsSpawnedByTask(ctx, sampleTask.Id, sampleTask.Execution, createdOrCreatingHostStatuses)
	assert.NoError(err)
	assert.Len(hosts, 5)

	// Error if there are more hosts than initially requested.
	handler.createHost.NumHosts = "1"
	assert.Equal(http.StatusBadRequest, handler.Run(ctx).Status())
}
