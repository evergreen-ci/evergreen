package data

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
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
			IP:     "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47",
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
		{
			Id:                 "7",
			ExternalIdentifier: "container-1234",
			Status:             evergreen.HostRunning,
			SpawnOptions: host.SpawnOptions{
				TaskID: "task_1",
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
	require.Len(found, 3)
	assert.Equal("4.com", found[0].Host)
	assert.Equal("1.com", found[1].Host)
	assert.Equal("container-1234", found[2].ExternalIdentifier)
	assert.Equal("abcd:1234:459c:2d00:cfe4:843b:1d60:8e47", found[1].IP)
}

func TestCreateHostsFromTask(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(task.Collection, model.VersionCollection, distro.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, host.Collection))
	t1 := task.Task{
		Id:           "t1",
		DisplayName:  "t1",
		Version:      "v1",
		DistroId:     "distro",
		Project:      "p",
		BuildVariant: "bv",
		HostId:       "h1",
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
	v1 := model.Version{
		Id:         "v1",
		Config:     versionYaml,
		Identifier: "p",
	}
	assert.NoError(v1.Insert())
	h1 := host.Host{
		Id:          "h1",
		RunningTask: t1.Id,
	}
	assert.NoError(h1.Insert())
	providerSettings := map[string]interface{}{
		"ami":      "ami-1234",
		"vpc_name": "my_vpc",
		"key_name": "myKey",
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
		settings := *h.Distro.ProviderSettings
		assert.NotEmpty(settings["key_name"])
		assert.InDelta(time.Now().Add(cloud.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
	}

	// test that a host.create in a function, expansions work
	assert.NoError(db.ClearCollections(host.Collection))
	versionYaml = `
functions:
  make-host:
    command: host.create
    params:
      distro: ${distro}
      scope: task
      num_hosts: 2
tasks:
- name: t2
  commands:
  - func: "make-host"
buildvariants:
- name: "bv"
  expansions:
    distro: distro
  tasks:
  - name: t2
`
	v2 := model.Version{
		Id:         "v2",
		Config:     versionYaml,
		Identifier: "p",
	}
	assert.NoError(v2.Insert())
	t2 := task.Task{
		Id:           "t2",
		DisplayName:  "t2",
		Version:      "v2",
		DistroId:     "distro",
		Project:      "p",
		BuildVariant: "bv",
		HostId:       "h2",
	}
	assert.NoError(t2.Insert())
	h2 := host.Host{
		Id:          "h2",
		RunningTask: t2.Id,
	}
	assert.NoError(h2.Insert())
	err = dc.CreateHostsFromTask(&t2, user.DBUser{Id: "me"}, "")
	assert.NoError(err)
	createdHosts, err = host.Find(host.IsUninitialized)
	assert.NoError(err)
	assert.Len(createdHosts, 2)
	for _, h := range createdHosts {
		assert.Equal("me", h.StartedBy)
		assert.True(h.UserHost)
		assert.Equal(t2.Id, h.ProvisionOptions.TaskId)
		settings := *h.Distro.ProviderSettings
		assert.NotEmpty(settings["key_name"])
		assert.InDelta(time.Now().Add(cloud.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
	}
}

func TestCreateContainerFromTask(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, model.VersionCollection, distro.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, host.Collection))
	t1 := task.Task{
		Id:           "t1",
		DisplayName:  "t1",
		Version:      "v1",
		DistroId:     "distro",
		Project:      "p",
		BuildVariant: "bv",
		HostId:       "h1",
	}
	assert.NoError(t1.Insert())
	versionYaml := `
tasks:
- name: t1
  commands:
  - command: host.create
    params:
      image: docker.io/library/hello-world
      distro: distro
      command: echo hi
      provider: docker
      num_hosts: 1
      background: false

buildvariants:
- name: "bv"
  tasks:
  - name: t1
`

	v1 := model.Version{
		Id:         "v1",
		Config:     versionYaml,
		Identifier: "p",
	}
	assert.NoError(v1.Insert())
	h1 := host.Host{
		Id:          "h1",
		RunningTask: t1.Id,
	}
	assert.NoError(h1.Insert())

	parent := distro.Distro{Id: "parent-distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}
	require.NoError(parent.Insert())

	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}
	parentHost := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	require.NoError(parentHost.Insert())

	d := distro.Distro{Id: "distro", Provider: evergreen.ProviderNameMock, ContainerPool: "test-pool"}
	require.NoError(d.Insert())

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
	require.Len(createdHosts, 1)
	h := createdHosts[0]
	assert.Equal("me", h.StartedBy)
	assert.Equal("docker.io/library/hello-world", h.DockerOptions.Image)
	assert.Equal("echo hi", h.DockerOptions.Command)
	assert.Empty("", h.ExternalIdentifier)
	assert.Equal(distro.DockerImageBuildTypePull, h.DockerOptions.Method)
}
