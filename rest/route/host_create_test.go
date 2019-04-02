package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/gimlet"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mitchellh/mapstructure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeIntentHost(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))
	handler := hostCreateHandler{
		sc: &data.DBConnector{},
	}

	d := distro.Distro{
		Id: "archlinux-test",
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-123456",
		},
	}
	require.NoError(d.Insert())

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
	h, err := handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings := &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Distro.Provider)
	assert.Equal("task-id", h.SpawnOptions.TaskID)
	assert.Equal("mock_key", ec2Settings.KeyName)
	assert.Equal(true, ec2Settings.IsVpc)

	// scope to build
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
	h, err = handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("build-id", h.SpawnOptions.BuildID)
	assert.Equal("mock_key", ec2Settings.KeyName)
	assert.Equal(true, ec2Settings.IsVpc)

	// spawn a spot evergreen distro
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Spot:                true,
		KeyName:             "mock_key",
	}
	handler.createHost = c
	h, err = handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Distro.Provider)
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
		Spot:                true,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		Subnet:              "subnet-123456",
	}
	handler.createHost = c
	h, err = handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Distro.Provider)
	assert.Equal("my_aws_key", ec2Settings.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)
	assert.Equal(true, ec2Settings.IsVpc)

	// bring your own ami
	c = apimodels.CreateHost{
		AMI:                 "ami-654321",
		CloudProvider:       "ec2",
		NumHosts:            "1",
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Spot:                true,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		Subnet:              "subnet-123456",
	}
	handler.createHost = c
	h, err = handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-654321", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Distro.Provider)
	assert.Equal("my_aws_key", ec2Settings.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)
	assert.Equal(true, ec2Settings.IsVpc)

}

func TestHostCreateDocker(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))
	handler := hostCreateHandler{
		sc: &data.DBConnector{},
	}

	d := distro.Distro{
		Id: "archlinux-test",
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-123456",
		},
	}
	require.NoError(d.Insert())

	c := apimodels.CreateHost{
		CloudProvider: apimodels.ProviderDocker,
		NumHosts:      "1",
		Distro:        "archlinux-test",
		Image:         "my-image",
		Command:       "echo hello",
	}
	c.Registry.Name = "myregistry"
	handler.createHost = c

	h, err := handler.sc.MakeIntentHost(handler.taskID, "", "", handler.createHost)
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	assert.Equal("my-image", h.DockerOptions.Image)
	assert.Equal("echo hello", h.DockerOptions.Command)
	assert.Equal("myregistry", h.DockerOptions.RegistryName)

	hosts, err := host.Find(db.Q{})
	assert.NoError(err)
	require.Len(hosts, 0)

	assert.Equal(200, handler.Run(context.Background()).Status())
	hosts, err = host.Find(db.Q{})
	assert.NoError(err)
	require.Len(hosts, 1)
	assert.Equal(h.DockerOptions.Command, hosts[0].DockerOptions.Command)

}

func TestGetDockerLogs(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))
	handler := containerLogsHandler{
		sc: &data.MockConnector{},
	}

	d := distro.Distro{
		Id: "archlinux-test",
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-123456",
		},
	}
	require.NoError(d.Insert())
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())

	h, err := handler.sc.MakeIntentHost("task-id", "", "", apimodels.CreateHost{
		Distro:        "archlinux-test",
		CloudProvider: "docker",
		NumHosts:      "1",
		Scope:         "task",
	})
	require.NoError(err)
	h.ParentID = "parent"
	h.ExternalIdentifier = "my-container"
	require.NoError(h.Insert())

	parent := host.Host{
		Id:            "parent",
		HasContainers: true,
	}
	require.NoError(parent.Insert())

	// invalid tail
	url := fmt.Sprintf("/hosts/%s/logs/output?tail=%s", h.Id, "invalid")
	request, err := http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)
	options := map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	url = fmt.Sprintf("/hosts/%s/logs/output?tail=%s", h.Id, "-1")
	request, err = http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)
	options = map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	// invalid Parse start time
	startTime := time.Now().Add(-time.Minute).String()
	url = fmt.Sprintf("/hosts/%s/logs/output?start_time=%s", h.Id, startTime)
	request, err = http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)
	options = map[string]string{"host_id": h.Id}

	request = gimlet.SetURLVars(request, options)
	assert.Error(handler.Parse(context.Background(), request))

	// valid Parse
	startTime = time.Now().Add(-time.Minute).Format(time.RFC3339)
	endTime := time.Now().Format(time.RFC3339)
	url = fmt.Sprintf("/hosts/%s/logs/output?start_time=%s&end_time=%s&tail=10", h.Id, startTime, endTime)

	request, err = http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)
	request = gimlet.SetURLVars(request, options)

	assert.NoError(handler.Parse(context.Background(), request))
	assert.Equal(h.Id, handler.host.Id)
	assert.Equal(startTime, handler.startTime)
	assert.Equal(endTime, handler.endTime)
	assert.Equal("10", handler.tail)

	// valid Run
	res := handler.Run(context.Background())
	require.NotNil(res)
	logs := res.Data().(*bytes.Buffer)
	assert.NoError(err)
	assert.Contains(logs.String(), "this is a log message")

}

func TestGetDockerLogsError(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))
	handler := containerLogsHandler{
		sc: &data.MockConnector{},
	}

	d := distro.Distro{
		Id: "archlinux-test",
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-123456",
		},
	}
	require.NoError(d.Insert())
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())

	h, err := handler.sc.MakeIntentHost("task-id", "", "", apimodels.CreateHost{
		Distro:        "archlinux-test",
		CloudProvider: "docker",
		NumHosts:      "1",
		Scope:         "task",
	})
	require.NoError(err)
	h.ParentID = "parent"
	require.NoError(h.Insert())

	parent := host.Host{
		Id:            "parent",
		HasContainers: true,
	}
	require.NoError(parent.Insert())

	url := fmt.Sprintf("/hosts/%s/logs/output", h.Id)

	request, err := http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)

	options := map[string]string{"host_id": h.Id}
	request = gimlet.SetURLVars(request, options)

	assert.NoError(handler.Parse(context.Background(), request))
	assert.Equal(h.Id, handler.host.Id)

	res := handler.Run(context.Background())
	require.NotNil(res)
	assert.Equal(http.StatusBadRequest, res.Status())
}

func TestGetDockerStatus(t *testing.T) {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection, task.Collection))
	handler := containerStatusHandler{
		sc: &data.MockConnector{},
	}

	d := distro.Distro{
		Id: "archlinux-test",
		ProviderSettings: &map[string]interface{}{
			"ami": "ami-123456",
		},
	}
	require.NoError(d.Insert())
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())

	h, err := handler.sc.MakeIntentHost("task-id", "", "", apimodels.CreateHost{
		Distro:        "archlinux-test",
		CloudProvider: "docker",
		NumHosts:      "1",
		Scope:         "task",
	})
	require.NoError(err)
	h.ParentID = "parent"
	h.ExternalIdentifier = "my-container"
	require.NoError(h.Insert())

	parent := host.Host{
		Id:            "parent",
		HasContainers: true,
	}
	require.NoError(parent.Insert())

	url := fmt.Sprintf("/hosts/%s/logs/status", h.Id)
	options := map[string]string{"host_id": h.Id}

	request, err := http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)
	request = gimlet.SetURLVars(request, options)

	assert.NoError(handler.Parse(context.Background(), request))
	require.NotNil(handler.host)
	assert.Equal(h.Id, handler.host.Id)

	// valid Run
	res := handler.Run(context.Background())
	require.NotNil(res)
	assert.Equal(http.StatusOK, res.Status())

	status, ok := res.Data().(*cloud.ContainerStatus)
	require.True(ok)
	require.NotNil(status)
	require.True(status.HasStarted)

}
