package data

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
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
			IPv4:   "12.34.56.78",
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
			Id:     "7",
			Status: evergreen.HostRunning,
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

	found, err := ListHostsForTask(context.Background(), "task_1")
	assert.NoError(err)
	require.Len(found, 3)
	assert.Equal("4.com", found[0].Host)
	assert.Equal("1.com", found[1].Host)
	assert.Equal("abcd:1234:459c:2d00:cfe4:843b:1d60:8e47", found[1].IP)
	assert.Equal("12.34.56.78", found[1].IPv4)
}

func TestCreateHostsFromTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection, distro.Collection, model.ProjectRefCollection, model.ProjectVarsCollection, host.Collection, model.ParserProjectCollection))
	settingsList := []*birch.Document{birch.NewDocument(
		birch.EC.String("region", "us-east-1"),
		birch.EC.String("ami", "ami-123456"),
		birch.EC.String("vpc_name", "my_vpc"),
		birch.EC.String("key_name", "myKey"),
		birch.EC.String("instance_type", "t1.micro"),
		birch.EC.SliceString("security_group_ids", []string{"sg-distro"}),
		birch.EC.String("subnet_id", "subnet-123456"),
	)}

	d := distro.Distro{
		Id:                   "distro",
		ProviderSettingsList: settingsList,
	}
	assert.NoError(t, d.Insert(ctx))
	p := model.ProjectRef{
		Id: "p",
	}
	assert.NoError(t, p.Insert())
	pvars := model.ProjectVars{
		Id: "p",
	}
	assert.NoError(t, pvars.Insert())

	env := &mock.Environment{}
	assert.NoError(t, env.Configure(ctx))
	env.EvergreenSettings.Credentials = map[string]string{"github": "token globalGitHubOauthToken"}
	var err error
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(t, err)

	// Run tests
	t.Run("Classic", func(t *testing.T) {
		versionYaml := `
tasks:
- name: t1
  commands:
  - command: host.create
    params:
      distro: distro
      scope: task
      num_hosts: 3
      security_group_ids: [sg-provided]
buildvariants:
- name: "bv"
  tasks:
  - name: t1
`
		v1 := model.Version{
			Id:         "v1",
			Identifier: "p",
		}
		assert.NoError(t, v1.Insert())
		t1 := task.Task{
			Id:           "t1",
			DisplayName:  "t1",
			Version:      "v1",
			DistroId:     "distro",
			Project:      "p",
			BuildVariant: "bv",
			HostId:       "h1",
		}
		assert.NoError(t, t1.Insert())
		h1 := host.Host{
			Id:          "h1",
			RunningTask: t1.Id,
		}
		assert.NoError(t, h1.Insert())
		pp := &model.ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(versionYaml), &pp)
		assert.NoError(t, err)
		pp.Id = "v1"
		assert.NoError(t, pp.Insert())

		assert.NoError(t, CreateHostsFromTask(ctx, env, &t1, user.DBUser{Id: "me"}, ""))
		createdHosts, err := host.FindWithContext(ctx, bson.M{host.StartedByKey: "me"})
		assert.NoError(t, err)
		assert.Len(t, createdHosts, 3)
		for _, h := range createdHosts {
			assert.Equal(t, "me", h.StartedBy)
			assert.True(t, h.UserHost)
			assert.Equal(t, t1.Id, h.ProvisionOptions.TaskId)
			assert.Len(t, h.Distro.ProviderSettingsList, 1)
			ec2Settings := &cloud.EC2ProviderSettings{}
			assert.NoError(t, ec2Settings.FromDistroSettings(h.Distro, ""))
			assert.NotEmpty(t, ec2Settings.KeyName)
			assert.InDelta(t, time.Now().Add(evergreen.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
			require.Len(t, ec2Settings.SecurityGroupIDs, 1)
			assert.Equal(t, "sg-provided", ec2Settings.SecurityGroupIDs[0])
			assert.Equal(t, distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")
			assert.Equal(t, h.Distro.Id, "distro")
		}
	})

	t.Run("InsideFunctionWithExpansions", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(host.Collection))
		versionYaml := `
functions:
  make-host:
    command: host.create
    params:
      distro: ${distro}
      scope: task
      num_hosts: 2
      security_group_ids: [sg-provided]
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
			Identifier: "p",
		}
		assert.NoError(t, v2.Insert())
		t2 := task.Task{
			Id:           "t2",
			DisplayName:  "t2",
			Version:      "v2",
			DistroId:     "distro",
			Project:      "p",
			BuildVariant: "bv",
			HostId:       "h2",
		}
		assert.NoError(t, t2.Insert())
		h2 := host.Host{
			Id:          "h2",
			RunningTask: t2.Id,
		}
		assert.NoError(t, h2.Insert())
		pp := &model.ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(versionYaml), &pp)
		assert.NoError(t, err)
		pp.Id = "v2"
		assert.NoError(t, pp.Insert())

		err = CreateHostsFromTask(ctx, env, &t2, user.DBUser{Id: "me"}, "")
		assert.NoError(t, err)
		createdHosts, err := host.FindWithContext(ctx, bson.M{host.StartedByKey: "me"})
		assert.NoError(t, err)
		assert.Len(t, createdHosts, 2)
		for _, h := range createdHosts {
			assert.Equal(t, "me", h.StartedBy)
			assert.True(t, h.UserHost)
			assert.Equal(t, t2.Id, h.ProvisionOptions.TaskId)
			assert.Len(t, h.Distro.ProviderSettingsList, 1)
			ec2Settings := &cloud.EC2ProviderSettings{}
			assert.NoError(t, ec2Settings.FromDistroSettings(h.Distro, ""))
			assert.NotEmpty(t, ec2Settings.KeyName)
			assert.InDelta(t, time.Now().Add(evergreen.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
			require.Len(t, ec2Settings.SecurityGroupIDs, 1)
			assert.Equal(t, "sg-provided", ec2Settings.SecurityGroupIDs[0])
			assert.Equal(t, distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")
			assert.Equal(t, h.Distro.Id, "distro")
		}
	})

	t.Run("WithCommandVars", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(host.Collection))
		versionYaml := `
functions:
  make-host:
    command: host.create
    params:
      distro: ${distro}
      scope: task
      num_hosts: 2
      security_group_ids: [sg-provided]
tasks:
- name: t3
  commands:
  - func: "make-host"
    vars:
      distro: distro
buildvariants:
- name: "bv"
  tasks:
  - name: t3
`
		v3 := model.Version{
			Id:         "v3",
			Identifier: "p",
		}
		assert.NoError(t, v3.Insert())
		t3 := task.Task{
			Id:           "t3",
			DisplayName:  "t3",
			Version:      "v3",
			DistroId:     "distro",
			Project:      "p",
			BuildVariant: "bv",
			HostId:       "h3",
		}
		assert.NoError(t, t3.Insert())
		h3 := host.Host{
			Id:          "h3",
			RunningTask: t3.Id,
		}
		assert.NoError(t, h3.Insert())
		pp := &model.ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(versionYaml), &pp)
		assert.NoError(t, err)
		pp.Id = "v3"
		assert.NoError(t, pp.Insert())

		settings := &evergreen.Settings{
			Credentials: map[string]string{"github": "token globalGitHubOauthToken"},
		}
		assert.NoError(t, evergreen.UpdateConfig(settings))

		assert.NoError(t, CreateHostsFromTask(ctx, env, &t3, user.DBUser{Id: "me"}, ""))
		createdHosts, err := host.FindWithContext(ctx, bson.M{host.StartedByKey: "me"})
		assert.NoError(t, err)
		assert.Len(t, createdHosts, 2)
		for _, h := range createdHosts {
			assert.Equal(t, "me", h.StartedBy)
			assert.True(t, h.UserHost)
			assert.Equal(t, t3.Id, h.ProvisionOptions.TaskId)
			assert.Len(t, h.Distro.ProviderSettingsList, 1)
			ec2Settings := &cloud.EC2ProviderSettings{}
			assert.NoError(t, ec2Settings.FromDistroSettings(h.Distro, ""))
			assert.NotEmpty(t, ec2Settings.KeyName)
			assert.InDelta(t, time.Now().Add(evergreen.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
			require.Len(t, ec2Settings.SecurityGroupIDs, 1)
			assert.Equal(t, "sg-provided", ec2Settings.SecurityGroupIDs[0])
			assert.Equal(t, distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")
			assert.Equal(t, h.Distro.Id, "distro")
		}
	})

	t.Run("SecurityGroupNotProvided", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(host.Collection))
		versionYaml := `
tasks:
- name: t4
  commands:
  - command: host.create
    params:
      distro: distro
      scope: task
      num_hosts: 3
buildvariants:
- name: "bv"
  tasks:
  - name: t4
`
		v4 := model.Version{
			Id:         "v4",
			Identifier: "p",
		}
		assert.NoError(t, v4.Insert())
		t4 := task.Task{
			Id:           "t4",
			DisplayName:  "t4",
			Version:      "v4",
			DistroId:     "distro",
			Project:      "p",
			BuildVariant: "bv",
			HostId:       "h4",
		}
		assert.NoError(t, t4.Insert())
		h4 := host.Host{
			Id:          "h4",
			RunningTask: t4.Id,
		}
		assert.NoError(t, h4.Insert())

		pp := &model.ParserProject{}
		err := util.UnmarshalYAMLWithFallback([]byte(versionYaml), &pp)
		assert.NoError(t, err)
		pp.Id = "v4"
		assert.NoError(t, pp.Insert())

		settings := &evergreen.Settings{
			Credentials: map[string]string{"github": "token globalGitHubOauthToken"},
		}
		assert.NoError(t, evergreen.UpdateConfig(settings))

		assert.NoError(t, CreateHostsFromTask(ctx, env, &t4, user.DBUser{Id: "me"}, ""))
		createdHosts, err := host.FindWithContext(ctx, bson.M{host.StartedByKey: "me"})
		assert.NoError(t, err)
		assert.Len(t, createdHosts, 3)
		for _, h := range createdHosts {
			assert.Equal(t, "me", h.StartedBy)
			assert.True(t, h.UserHost)
			assert.Equal(t, t4.Id, h.ProvisionOptions.TaskId)
			assert.Len(t, h.Distro.ProviderSettingsList, 1)
			ec2Settings := &cloud.EC2ProviderSettings{}
			assert.NoError(t, ec2Settings.FromDistroSettings(h.Distro, ""))
			assert.NotEmpty(t, ec2Settings.KeyName)
			assert.InDelta(t, time.Now().Add(evergreen.DefaultSpawnHostExpiration).Unix(), h.ExpirationTime.Unix(), float64(1*time.Millisecond))
			require.Len(t, ec2Settings.SecurityGroupIDs, 2)
			assert.Equal(t, "sg-distro", ec2Settings.SecurityGroupIDs[0]) // if not overridden, stick with ec2 security group
			assert.Equal(t, distro.BootstrapMethodNone, h.Distro.BootstrapSettings.Method, "host provisioning should be set to none by default")
			assert.Equal(t, h.Distro.Id, "distro")
		}
	})
}

func TestCreateContainerFromTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(task.Collection, model.VersionCollection, distro.Collection, model.ProjectRefCollection,
		model.ProjectVarsCollection, host.Collection, model.ParserProjectCollection))

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
      environment_vars:
          apple: red
          banana: yellow

buildvariants:
- name: "bv"
  tasks:
  - name: t1
`

	v1 := model.Version{
		Id:         "v1",
		Identifier: "p",
	}
	assert.NoError(v1.Insert())
	h1 := host.Host{
		Id:          "h1",
		RunningTask: t1.Id,
	}
	assert.NoError(h1.Insert())
	pp := model.ParserProject{}
	err := util.UnmarshalYAMLWithFallback([]byte(versionYaml), &pp)
	require.NoError(err)
	pp.Id = "v1"
	require.NoError(pp.Insert())

	parent := distro.Distro{
		Id:       "parent-distro",
		Provider: evergreen.ProviderNameDockerMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
	require.NoError(parent.Insert(ctx))

	pool := evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	env.EvergreenSettings.ContainerPools = evergreen.ContainerPoolsConfig{Pools: []evergreen.ContainerPool{pool}}
	env.EvergreenSettings.Credentials = map[string]string{"github": "token globalGitHubOauthToken"}
	env.RemoteGroup, err = queue.NewLocalQueueGroup(ctx, queue.LocalQueueGroupOptions{
		DefaultQueue: queue.LocalQueueOptions{Constructor: func(context.Context) (amboy.Queue, error) {
			return queue.NewLocalLimitedSize(2, 1048), nil
		}}})
	assert.NoError(err)

	parentHost := &host.Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: &pool,
	}
	require.NoError(parentHost.Insert())

	d := distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameDockerMock,
		ContainerPool: pool.Id,
	}
	require.NoError(d.Insert(ctx))

	p := model.ProjectRef{
		Id: "p",
	}
	assert.NoError(p.Insert())
	pvars := model.ProjectVars{
		Id: "p",
	}
	assert.NoError(pvars.Insert())

	assert.NoError(CreateHostsFromTask(ctx, env, &t1, user.DBUser{Id: "me"}, ""))

	createdHosts, err := host.FindWithContext(ctx, bson.M{host.StartedByKey: "me"})
	assert.NoError(err)
	require.Len(createdHosts, 1)
	h := createdHosts[0]
	assert.Equal("me", h.StartedBy)
	assert.Equal("docker.io/library/hello-world", h.DockerOptions.Image)
	assert.Equal("echo hi", h.DockerOptions.Command)
	assert.Equal(distro.DockerImageBuildTypePull, h.DockerOptions.Method)
	assert.Len(h.DockerOptions.EnvironmentVars, 2)

	foundApple := false
	foundBanana := false
	for _, envVar := range h.DockerOptions.EnvironmentVars {
		if envVar == "banana=yellow" {
			foundBanana = true
		} else if envVar == "apple=red" {
			foundApple = true
		}
	}
	assert.True(foundApple)
	assert.True(foundBanana)
}
