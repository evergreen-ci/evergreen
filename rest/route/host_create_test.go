package route

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
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
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
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
	require.NoError(sampleTask.Insert())

	// spawn an evergreen distro
	c := apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		KeyName:             "mock_key",
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
	assert.Equal("mock_key", ec2Settings.KeyName)
	assert.Equal(true, ec2Settings.IsVpc)

	// test roundtripping
	h, err = host.FindOneByIdOrTag(ctx, h.Id)
	assert.NoError(err)
	require.NotNil(h)
	ec2Settings2 := &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings2.AMI)
	assert.Equal("mock_key", ec2Settings2.KeyName)
	assert.Equal(true, ec2Settings2.IsVpc)

	// scope to build
	require.NoError(db.ClearCollections(task.Collection))
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "build",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		KeyName:             "mock_key",
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
	assert.Equal("mock_key", ec2Settings.KeyName)
	assert.Equal(true, ec2Settings.IsVpc)

	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal(distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")

	// Using an alias should resolve to the actual distro
	c = apimodels.CreateHost{
		Distro:              "archlinux-alias",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		KeyName:             "mock_key",
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
	assert.Equal("mock_key", ec2Settings.KeyName)
	assert.Equal(true, ec2Settings.IsVpc)

	// override some evergreen distro settings
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		Subnet:              "subnet-123456",
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
	assert.Equal("my_aws_key", ec2Settings.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)
	assert.Equal(true, ec2Settings.IsVpc)

	ec2Settings2 = &cloud.EC2ProviderSettings{}
	require.Len(h.Distro.ProviderSettingsList, 1)
	assert.NoError(ec2Settings2.FromDistroSettings(h.Distro, ""))
	assert.Equal("ami-123456", ec2Settings2.AMI)
	assert.Equal("my_aws_key", ec2Settings2.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings2.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings2.SubnetId)
	assert.Equal(true, ec2Settings2.IsVpc)

	// bring your own ami
	c = apimodels.CreateHost{
		AMI:                 "ami-654321",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		InstanceType:        "t1.micro",
		Subnet:              "subnet-123456",
		SecurityGroups:      []string{"1234"},
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
	assert.Equal("my_aws_key", ec2Settings2.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings2.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings2.SubnetId)
	assert.Equal(true, ec2Settings2.IsVpc)

	// with multiple regions
	require.Len(d.ProviderSettingsList, 1)
	doc2 := d.ProviderSettingsList[0].Copy().Set(birch.EC.String("region", "us-west-1")).Set(birch.EC.String("ami", "ami-987654"))
	d.ProviderSettingsList = append(d.ProviderSettingsList, doc2)
	require.NoError(d.ReplaceOne(ctx))
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		KeyName:             "mock_key",
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
	assert.Equal(ec2Settings2.AMI, "ami-123456")

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
	assert.Equal(ec2Settings2.AMI, "ami-987654")
}

func TestHostCreateDocker(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection, evergreen.ConfigCollection))
	pool := evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	env.EvergreenSettings.ContainerPools = evergreen.ContainerPoolsConfig{Pools: []evergreen.ContainerPool{pool}}

	handler := hostCreateHandler{env: env}

	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameDockerMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
		ContainerPool: pool.Id,
	}
	require.NoError(parent.Insert(ctx))

	parentHost := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: &pool,
	}
	require.NoError(parentHost.Insert(ctx))

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameDockerMock, ContainerPool: "test-pool"}
	require.NoError(d.Insert(ctx))

	sampleTask := &task.Task{
		Id: handler.taskID,
	}
	require.NoError(sampleTask.Insert())

	extraHosts := []string{"localhost:127.0.0.1"}
	c := apimodels.CreateHost{
		CloudProvider:     apimodels.ProviderDocker,
		NumHosts:          "1",
		Distro:            "distro",
		Image:             "my-image",
		Command:           "echo hello",
		StdinFileContents: []byte("hello!"),
		EnvironmentVars:   map[string]string{"env_key": "env_value"},
		ExtraHosts:        extraHosts,
	}
	c.Registry.Name = "myregistry"
	handler.createHost = c

	foundDistro, err := distro.GetHostCreateDistro(ctx, c)
	require.NoError(err)
	h, err := data.MakeHost(ctx, env, handler.taskID, "", "", handler.createHost, *foundDistro)
	assert.NoError(err)
	require.NotNil(h)
	assert.Equal("distro", h.Distro.Id)
	assert.Equal("my-image", h.DockerOptions.Image)
	assert.Equal("echo hello", h.DockerOptions.Command)
	assert.Equal("hello!", string(h.DockerOptions.StdinData))
	assert.Equal("myregistry", h.DockerOptions.RegistryName)
	assert.Equal([]string{"env_key=env_value"}, h.DockerOptions.EnvironmentVars)
	assert.Equal(extraHosts, h.DockerOptions.ExtraHosts)

	handler.distro = *foundDistro
	assert.Equal(http.StatusOK, handler.Run(ctx).Status())

	hosts, err := host.Find(ctx, bson.M{})
	assert.NoError(err)
	require.Len(hosts, 3)
	assert.Equal(h.DockerOptions.Command, hosts[1].DockerOptions.Command)
}

func TestGetDockerLogs(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection, evergreen.ConfigCollection))
	handler := containerLogsHandler{}
	pool := evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	env.EvergreenSettings.ContainerPools = evergreen.ContainerPoolsConfig{Pools: []evergreen.ContainerPool{pool}}
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(err)

	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
		ContainerPool: pool.Id,
	}
	require.NoError(parent.Insert(ctx))

	parentHost := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: &pool,
	}
	require.NoError(parentHost.Insert(ctx))

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameDockerMock, ContainerPool: "test-pool"}
	require.NoError(d.Insert(ctx))

	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())
	c := apimodels.CreateHost{
		CloudProvider: apimodels.ProviderDocker,
		NumHosts:      "1",
		Distro:        "distro",
		Image:         "my-image",
		Command:       "echo hello",
	}
	h, err := data.MakeHost(ctx, env, "task-id", "", "", c, d)
	require.NoError(err)
	require.NotNil(h)
	assert.NotEmpty(h.ParentID)

	// invalid tail
	url := fmt.Sprintf("/hosts/%s/logs/output?tail=%s", h.Id, "invalid")
	request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
	assert.NoError(err)
	options := map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	url = fmt.Sprintf("/hosts/%s/logs/output?tail=%s", h.Id, "-1")
	request, err = http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
	assert.NoError(err)
	options = map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	// invalid Parse start time
	startTime := time.Now().Add(-time.Minute).String()
	url = fmt.Sprintf("/hosts/%s/logs/output?start_time=%s", h.Id, startTime)
	request, err = http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
	assert.NoError(err)
	options = map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	// valid Parse
	startTime = time.Now().Add(-time.Minute).Format(time.RFC3339)
	endTime := time.Now().Format(time.RFC3339)
	url = fmt.Sprintf("/hosts/%s/logs/output?start_time=%s&end_time=%s&tail=10", h.Id, startTime, endTime)

	request, err = http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
	assert.NoError(err)
	request = gimlet.SetURLVars(request, options)

	assert.NoError(handler.Parse(context.Background(), request))
	assert.Equal(h.Id, handler.host.Id)
	assert.Equal(startTime, handler.startTime)
	assert.Equal(endTime, handler.endTime)
	assert.Equal("10", handler.tail)

	// valid Run
	cloudClient := cloud.GetMockClient()
	logs, err := cloudClient.GetDockerLogs(nil, "containerId", handler.host, types.ContainerLogsOptions{})
	assert.NoError(err)
	buf := new(strings.Builder)
	_, err = io.Copy(buf, logs)
	assert.NoError(err)
	assert.Contains(buf.String(), "this is a log message")

}

func TestGetDockerStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection, evergreen.ConfigCollection))
	handler := containerStatusHandler{}
	pool := evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	env.EvergreenSettings.ContainerPools = evergreen.ContainerPoolsConfig{Pools: []evergreen.ContainerPool{pool}}
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(err)

	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameDockerMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
		ContainerPool: pool.Id,
	}
	require.NoError(parent.Insert(ctx))

	parentHost := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: &pool,
	}
	require.NoError(parentHost.Insert(ctx))

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameDockerMock, ContainerPool: "test-pool"}
	require.NoError(d.Insert(ctx))

	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())
	c := apimodels.CreateHost{
		CloudProvider: apimodels.ProviderDocker,
		NumHosts:      "1",
		Distro:        "distro",
		Image:         "my-image",
		Command:       "echo hello",
	}
	h, err := data.MakeHost(ctx, env, "task-id", "", "", c, d)
	require.NoError(err)
	assert.NotEmpty(h.ParentID)

	url := fmt.Sprintf("/hosts/%s/logs/status", h.Id)
	options := map[string]string{"host_id": h.Id}

	request, err := http.NewRequest(http.MethodGet, url, bytes.NewReader(nil))
	assert.NoError(err)
	request = gimlet.SetURLVars(request, options)

	assert.NoError(handler.Parse(context.Background(), request))
	require.NotNil(handler.host)
	assert.Equal(h.Id, handler.host.Id)

	// valid Run
	cloudClient := cloud.GetMockClient()
	status, err := cloudClient.GetDockerStatus(nil, "containerId", handler.host)
	assert.NoError(err)
	require.NotNil(status)
	require.True(status.HasStarted)

}
