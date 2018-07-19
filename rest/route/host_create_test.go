package route

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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
	handler := hostCreateHandler{}

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
		NumHosts:            1,
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	handler.taskID = "task-id"
	h, err := handler.makeIntentHost()
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings := &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2OnDemand, h.Provider)
	assert.Equal("task-id", h.SpawnOptions.TaskID)

	// scope to build
	myTask := task.Task{
		Id:      "task-id",
		BuildId: "build-id",
	}
	require.NoError(myTask.Insert())
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            1,
		Scope:               "build",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
	}
	handler.createHost = c
	handler.taskID = "task-id"
	h, err = handler.makeIntentHost()
	assert.NoError(err)
	assert.NotNil(h)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("build-id", h.SpawnOptions.BuildID)

	// spawn a spot evergreen distro
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            1,
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Spot:                true,
	}
	handler.createHost = c
	h, err = handler.makeIntentHost()
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)

	// override some evergreen distro settings
	c = apimodels.CreateHost{
		Distro:              "archlinux-test",
		CloudProvider:       "ec2",
		NumHosts:            1,
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Spot:                true,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		Subnet:              "subnet-123456",
	}
	handler.createHost = c
	h, err = handler.makeIntentHost()
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("archlinux-test", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-123456", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)
	assert.Equal("my_aws_key", ec2Settings.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)

	// bring your own ami
	c = apimodels.CreateHost{
		AMI:                 "ami-654321",
		CloudProvider:       "ec2",
		NumHosts:            1,
		Scope:               "task",
		SetupTimeoutSecs:    600,
		TeardownTimeoutSecs: 21600,
		Spot:                true,
		AWSKeyID:            "my_aws_key",
		AWSSecret:           "my_secret_key",
		Subnet:              "subnet-123456",
	}
	handler.createHost = c
	h, err = handler.makeIntentHost()
	assert.NoError(err)
	assert.NotNil(h)
	assert.Equal("", h.Distro.Id)
	ec2Settings = &cloud.EC2ProviderSettings{}
	err = mapstructure.Decode(h.Distro.ProviderSettings, ec2Settings)
	assert.NoError(err)
	assert.Equal("ami-654321", ec2Settings.AMI)
	assert.Equal(evergreen.ProviderNameEc2Spot, h.Provider)
	assert.Equal("my_aws_key", ec2Settings.AWSKeyID)
	assert.Equal("my_secret_key", ec2Settings.AWSSecret)
	assert.Equal("subnet-123456", ec2Settings.SubnetId)
}
