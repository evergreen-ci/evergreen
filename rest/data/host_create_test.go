package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListHostsForTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(host.Collection, build.Collection, task.Collection))
	hosts := []*host.Host{
		{
			Id:     "1",
			Host:   "1.com",
			Status: evergreen.HostRunning,
			SpawnOptions: host.SpawnOptions{
				TaskID: "task_1",
			},
		},
		{
			Id:     "2",
			Host:   "2.com",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "3",
			Host:   "3.com",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "4",
			Host:   "4.com",
			Status: evergreen.HostRunning,
			SpawnOptions: host.SpawnOptions{
				BuildID: "build_1",
			},
		},
		{
			Id:     "5",
			Host:   "5.com",
			Status: evergreen.HostDecommissioned,
			SpawnOptions: host.SpawnOptions{
				TaskID: "task_1",
			},
		},
		{
			Id:     "6",
			Host:   "6.com",
			Status: evergreen.HostTerminated,
			SpawnOptions: host.SpawnOptions{
				BuildID: "build_1",
			},
		},
	}
	for i := range hosts {
		require.NoError(hosts[i].Insert())
	}
	require.NoError((&task.Task{Id: "task_1", BuildId: "build_1"}).Insert())
	require.NoError((&build.Build{Id: "build_1"}).Insert())

	c := DBCreateHostConnector{}
	found, err := c.ListHostsForTask("task_1")
	assert.NoError(err)
	assert.Len(found, 2)
	assert.Equal("4.com", found[0].Host)
	assert.Equal("1.com", found[1].Host)
}

func TestCreateHostsFromTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, version.Collection, distro.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, host.Collection))
	t1 := task.Task{
		Id:           "t1",
		DisplayName:  "t1",
		Version:      "v1",
		DistroId:     "distro",
		Project:      "p",
		BuildVariant: "bv",
	}
	assert.NoError(t1.Insert())
	versionYaml := `
tasks:
- name: t1
  commands:
  - command: host.create
    params:
      distro: distro
      scope: task
      num_hosts: 3
buildvariants:
- name: "bv"
  tasks:
  - name: t1
`
	v1 := version.Version{
		Id:         "v1",
		Config:     versionYaml,
		Identifier: "p",
	}
	assert.NoError(v1.Insert())
	providerSettings := map[string]interface{}{
		"ami":      "ami-1234",
		"vpc_name": "my_vpc",
	}
	d := distro.Distro{
		Id:               "distro",
		ProviderSettings: &providerSettings,
	}
	assert.NoError(d.Insert())
	p := model.ProjectRef{
		Identifier: "p",
	}
	assert.NoError(p.Insert())
	pvars := model.ProjectVars{
		Id: "p",
	}
	assert.NoError(pvars.Insert())

	dc := DBCreateHostConnector{}
	err := dc.CreateHostsFromTask(&t1, user.DBUser{Id: "me"}, "")
	assert.NoError(err)

	createdHosts, err := host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(createdHosts, 3)
	for _, h := range createdHosts {
		assert.Equal("me", h.StartedBy)
		assert.True(h.UserHost)
		assert.Equal(t1.Id, h.ProvisionOptions.TaskId)
	}
}
