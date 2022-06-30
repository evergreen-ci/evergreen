package pod

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	testutil.Setup()
}

func TestOSValidate(t *testing.T) {
	t.Run("SucceedsForValidOperatingSystems", func(t *testing.T) {
		for _, os := range validOperatingSystems {
			assert.NoError(t, os.Validate())
		}
	})
	t.Run("FailsWithInvalidOperatingSystem", func(t *testing.T) {
		assert.Error(t, OS("invalid").Validate())
	})
	t.Run("FailsWithEmptyOperatingSystem", func(t *testing.T) {
		assert.Error(t, OS("").Validate())
	})
}

func TestImportOS(t *testing.T) {
	t.Run("SucceedsForValidContainerOperatingSystems", func(t *testing.T) {
		for _, os := range evergreen.ValidContainerOperatingSystems {
			imported, err := ImportOS(os)
			require.NoError(t, err)
			assert.NotZero(t, imported)
		}
	})
	t.Run("FailsWithInvalidContainerOperatingSystem", func(t *testing.T) {
		imported, err := ImportOS(evergreen.ContainerOS("invalid"))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
	t.Run("FailsWithEmptyOperatingSystem", func(t *testing.T) {
		imported, err := ImportOS(evergreen.ContainerOS(""))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
}

func TestArchValidate(t *testing.T) {
	t.Run("SucceedsForValidArchitectures", func(t *testing.T) {
		for _, arch := range validArchitectures {
			assert.NoError(t, arch.Validate())
		}
	})
	t.Run("FailsWithInvalidArchitecture", func(t *testing.T) {
		assert.Error(t, Arch("invalid").Validate())
	})
	t.Run("FailsWithEmptyArchitecture", func(t *testing.T) {
		assert.Error(t, Arch("").Validate())
	})
}

func TestImportArch(t *testing.T) {
	t.Run("SucceedsForValidContainerArchitecture", func(t *testing.T) {
		for _, arch := range evergreen.ValidContainerArchitectures {
			imported, err := ImportArch(arch)
			require.NoError(t, err)
			assert.NotZero(t, imported)
		}
	})
	t.Run("FailsWithInvalidContainerArchitecture", func(t *testing.T) {
		imported, err := ImportArch(evergreen.ContainerArch("invalid"))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
	t.Run("FailsWithEmptyArchitecture", func(t *testing.T) {
		imported, err := ImportArch(evergreen.ContainerArch(""))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
}

func TestWindowsVersionValidate(t *testing.T) {
	t.Run("SucceedsForValidWindowsVersions", func(t *testing.T) {
		for _, winVer := range validWindowsVersions {
			assert.NoError(t, winVer.Validate())
		}
	})
	t.Run("FailsWithInvalidWindowsVersion", func(t *testing.T) {
		assert.Error(t, WindowsVersion("invalid").Validate())
	})
	t.Run("FailsWithEmptyWindowsVersion", func(t *testing.T) {
		assert.Error(t, WindowsVersion("").Validate())
	})
}

func TestImportWindowsVersion(t *testing.T) {
	t.Run("SucceedsWithValidContainerWindowsVersions", func(t *testing.T) {
		for _, winVer := range evergreen.ValidWindowsVersions {
			imported, err := ImportWindowsVersion(winVer)
			require.NoError(t, err)
			assert.NotZero(t, imported)
		}
	})
	t.Run("FailsWithInvalidContainerWindowsVersion", func(t *testing.T) {
		imported, err := ImportWindowsVersion(evergreen.WindowsVersion("invalid"))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
	t.Run("FailsWithEmptyArchitecture", func(t *testing.T) {
		imported, err := ImportWindowsVersion(evergreen.WindowsVersion(""))
		assert.Error(t, err)
		assert.Zero(t, imported)
	})
}

func TestInsertAndFindOneByID(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"FindOneByIDReturnsNoResultForNonexistentPod": func(t *testing.T) {
			p, err := FindOneByID("foo")
			assert.NoError(t, err)
			assert.Zero(t, p)
		},
		"InsertSucceedsAndIsFoundByID": func(t *testing.T) {
			p := Pod{
				ID:     "id",
				Status: StatusRunning,
				TaskContainerCreationOpts: TaskContainerCreationOptions{
					Image:          "alpine",
					MemoryMB:       128,
					CPU:            128,
					OS:             OSWindows,
					Arch:           ArchAMD64,
					WindowsVersion: WindowsVersionServer2019,
				},
				TimeInfo: TimeInfo{
					Initializing:     utility.BSONTime(time.Now()),
					Starting:         utility.BSONTime(time.Now()),
					LastCommunicated: utility.BSONTime(time.Now()),
				},
				Resources: ResourceInfo{
					ExternalID:   "external_id",
					DefinitionID: "definition_id",
					Cluster:      "cluster",
					Containers: []ContainerResourceInfo{
						{
							ExternalID: "container_id",
							Name:       "container_name",
							SecretIDs:  []string{"secret0", "secret1"},
						},
					},
				},
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotNil(t, dbPod)

			assert.Equal(t, p.ID, dbPod.ID)
			assert.Equal(t, p.Status, dbPod.Status)
			assert.Equal(t, p.Resources, dbPod.Resources)
			assert.Equal(t, p.TimeInfo, dbPod.TimeInfo)
			assert.Equal(t, p.TaskContainerCreationOpts, dbPod.TaskContainerCreationOpts)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			tCase(t)
		})
	}
}

func TestRemove(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"NoopsWithNonexistentPod": func(t *testing.T) {
			p := Pod{
				ID: "nonexistent",
			}
			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)

			assert.NoError(t, p.Remove())
		},
		"SucceedsWithExistingPod": func(t *testing.T) {
			p := Pod{
				ID: "id",
			}
			require.NoError(t, p.Insert())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotNil(t, dbPod)

			require.NoError(t, p.Remove())

			dbPod, err = FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			tCase(t)
		})
	}
}

func TestNewTaskIntentPod(t *testing.T) {
	makeValidOpts := func() TaskIntentPodOptions {
		return TaskIntentPodOptions{
			ID:             "id",
			Secret:         "secret",
			CPU:            128,
			MemoryMB:       256,
			OS:             OSWindows,
			Arch:           ArchAMD64,
			WindowsVersion: WindowsVersionServer2022,
			Image:          "image",
			WorkingDir:     "/",
			RepoUsername:   "username",
			RepoPassword:   "password",
		}
	}
	t.Run("SucceedsWithValidOptions", func(t *testing.T) {
		opts := makeValidOpts()

		p, err := NewTaskIntentPod(opts)
		require.NoError(t, err)
		assert.Equal(t, opts.ID, p.ID)
		assert.Equal(t, opts.CPU, p.TaskContainerCreationOpts.CPU)
		assert.Equal(t, opts.MemoryMB, p.TaskContainerCreationOpts.MemoryMB)
		assert.Equal(t, opts.OS, p.TaskContainerCreationOpts.OS)
		assert.Equal(t, opts.Arch, p.TaskContainerCreationOpts.Arch)
		assert.Equal(t, opts.WindowsVersion, p.TaskContainerCreationOpts.WindowsVersion)
		assert.Equal(t, opts.Image, p.TaskContainerCreationOpts.Image)
		assert.Equal(t, opts.WorkingDir, p.TaskContainerCreationOpts.WorkingDir)
		assert.Equal(t, opts.RepoUsername, p.TaskContainerCreationOpts.RepoUsername)
		assert.Equal(t, opts.RepoPassword, p.TaskContainerCreationOpts.RepoPassword)
		assert.Equal(t, opts.ID, p.TaskContainerCreationOpts.EnvVars[PodIDEnvVar])
		s, err := p.GetSecret()
		require.NoError(t, err)
		assert.Zero(t, s.Name)
		assert.Equal(t, opts.Secret, s.Value)
		assert.Empty(t, s.ExternalID)
		assert.False(t, utility.FromBoolPtr(s.Exists))
		assert.True(t, utility.FromBoolPtr(s.Owned))
	})
	t.Run("SetsDefaultID", func(t *testing.T) {
		opts := makeValidOpts()
		opts.ID = ""

		p, err := NewTaskIntentPod(opts)
		require.NoError(t, err)
		assert.NotZero(t, p.ID)
		assert.Equal(t, p.ID, p.TaskContainerCreationOpts.EnvVars[PodIDEnvVar])
	})
	t.Run("SetsDefaultPodSecret", func(t *testing.T) {
		opts := makeValidOpts()
		opts.Secret = ""

		p, err := NewTaskIntentPod(opts)
		require.NoError(t, err)
		assert.NotZero(t, p.ID)
		s, err := p.GetSecret()
		require.NoError(t, err)
		assert.Zero(t, s.Name)
		assert.NotZero(t, s.Value)
		assert.Empty(t, s.ExternalID)
		assert.False(t, utility.FromBoolPtr(s.Exists))
		assert.True(t, utility.FromBoolPtr(s.Owned))
	})
	t.Run("FailsWithInvalidOptions", func(t *testing.T) {
		opts := makeValidOpts()
		opts.Image = ""

		p, err := NewTaskIntentPod(opts)
		assert.Error(t, err)
		assert.Zero(t, p)
	})
}

func TestUpdateStatus(t *testing.T) {
	checkStatus := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Status, dbPod.Status)
	}

	checkStatusAndTimeInfo := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Status, dbPod.Status)
		switch p.Status {
		case StatusInitializing:
			assert.NotZero(t, p.TimeInfo.Initializing)
			assert.NotZero(t, dbPod.TimeInfo.Initializing)
			assert.Equal(t, p.TimeInfo.Initializing, dbPod.TimeInfo.Initializing)
		case StatusStarting:
			assert.NotZero(t, p.TimeInfo.Starting)
			assert.NotZero(t, dbPod.TimeInfo.Starting)
			assert.Equal(t, p.TimeInfo.Starting, dbPod.TimeInfo.Starting)
		}
	}

	checkEventLog := func(t *testing.T, p Pod) {
		events, err := event.Find(event.MostRecentPodEvents(p.ID, 10))
		require.NoError(t, err)
		require.Len(t, events, 1)
		assert.Equal(t, p.ID, events[0].ResourceId)
		assert.Equal(t, event.ResourceTypePod, events[0].ResourceType)
		assert.Equal(t, string(event.EventPodStatusChange), events[0].EventType)
	}

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"SucceedsWithInitializingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusInitializing))
			assert.Equal(t, StatusInitializing, p.Status)

			checkStatusAndTimeInfo(t, p)
			checkEventLog(t, p)
		},
		"SucceedsWithStartingStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusStarting))
			assert.Equal(t, StatusStarting, p.Status)

			checkStatusAndTimeInfo(t, p)
			checkEventLog(t, p)
		},
		"SucceedsWithTerminatedStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(StatusTerminated))
			assert.Equal(t, StatusTerminated, p.Status)

			checkStatus(t, p)
			checkEventLog(t, p)
		},
		"NoopsWithIdenticalStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateStatus(p.Status))
			checkStatus(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			assert.Error(t, p.UpdateStatus(StatusTerminated))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
		"FailsWithChangedPodStatus": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, UpdateOne(ByID(p.ID), bson.M{
				"$set": bson.M{StatusKey: StatusInitializing},
			}))

			assert.Error(t, p.UpdateStatus(StatusTerminated))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, StatusInitializing, dbPod.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			}()
			tCase(t, Pod{
				ID:     "id",
				Status: StatusRunning,
			})
		})
	}
}

func TestUpdateResources(t *testing.T) {
	checkResources := func(t *testing.T, p Pod) {
		dbPod, err := FindOneByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPod)
		assert.Equal(t, p.Resources, dbPod.Resources)
	}

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"SucceedsForEmptyResources": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, p.UpdateResources(p.Resources))
			checkResources(t, p)
		},
		"SuccessWithCluster": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				Cluster: "cluster",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "cluster", p.Resources.Cluster)

			checkResources(t, p)
		},
		"SuccessWithExternalID": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				ExternalID: "external",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "external", p.Resources.ExternalID)

			checkResources(t, p)
		},
		"SuccessWithDefinitionID": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			info := ResourceInfo{
				DefinitionID: "definition",
			}
			require.NoError(t, p.UpdateResources(info))
			assert.Equal(t, "definition", p.Resources.DefinitionID)

			checkResources(t, p)
		},
		"SuccessWithContainers": func(t *testing.T, p Pod) {
			require.NoError(t, p.Insert())

			containerInfo := ContainerResourceInfo{
				ExternalID: "container_id",
				Name:       "container_name",
				SecretIDs:  []string{"secret"},
			}
			info := ResourceInfo{
				Containers: []ContainerResourceInfo{containerInfo},
			}
			require.NoError(t, p.UpdateResources(info))
			require.Len(t, p.Resources.Containers, 1)
			assert.Equal(t, containerInfo.ExternalID, p.Resources.Containers[0].ExternalID)
			assert.Equal(t, containerInfo.Name, p.Resources.Containers[0].Name)
			assert.Equal(t, containerInfo.SecretIDs, p.Resources.Containers[0].SecretIDs)

			checkResources(t, p)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			assert.Error(t, p.UpdateResources(ResourceInfo{}))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, event.LegacyEventLogCollection))
			}()
			tCase(t, Pod{
				ID:     "id",
				Status: StatusRunning,
				Resources: ResourceInfo{
					ExternalID:   utility.RandomString(),
					DefinitionID: utility.RandomString(),
					Cluster:      utility.RandomString(),
					Containers: []ContainerResourceInfo{
						{
							ExternalID: utility.RandomString(),
							Name:       utility.RandomString(),
							SecretIDs:  []string{utility.RandomString()},
						},
					},
				},
			})
		})
	}
}

func TestGetSecret(t *testing.T) {
	t.Run("SucceedsWithPopulatedEnvSecret", func(t *testing.T) {
		expected := Secret{
			Name:       "secret_name",
			Value:      "secret_value",
			ExternalID: "external_id",
		}
		p := Pod{
			ID: "id",
			TaskContainerCreationOpts: TaskContainerCreationOptions{
				EnvSecrets: map[string]Secret{
					PodSecretEnvVar: expected,
				},
			},
		}
		s, err := p.GetSecret()
		require.NoError(t, err)
		require.NotZero(t, s)
		assert.Equal(t, expected, *s)
	})
	t.Run("FailsWithoutSecret", func(t *testing.T) {
		var p Pod
		s, err := p.GetSecret()
		assert.Error(t, err)
		assert.Zero(t, s)
	})
}

func TestSetRunningTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			require.NoError(t, p.Insert())
			taskID := "task"
			require.NoError(t, p.SetRunningTask(ctx, env, taskID))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, taskID, dbPod.RunningTask)
		},
		"NoopsWithPodAlreadyRunningSameTask": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			const taskID = "task"
			p.RunningTask = taskID
			require.NoError(t, p.Insert())
			assert.NoError(t, p.SetRunningTask(ctx, env, taskID))

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, taskID, dbPod.RunningTask)
		},
		"FailsWithNonRunningPod": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			p.Status = StatusDecommissioned
			require.NoError(t, p.Insert())
			assert.Error(t, p.SetRunningTask(ctx, env, "task"))
		},
		"FailsWithPodAlreadyRunningDifferentTask": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			p.RunningTask = "some-other-task"
			require.NoError(t, p.Insert())
			assert.Error(t, p.SetRunningTask(ctx, env, "task"))
		},
		"FailsWithNonexistentPod": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			assert.Error(t, p.SetRunningTask(ctx, env, "task"))

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.ClearCollections(Collection))

			p := Pod{
				ID:     "id",
				Status: StatusRunning,
			}
			tCase(tctx, t, env, p)
		})
	}
}

func TestClearRunningTask(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, p Pod){
		"Succeeds": func(t *testing.T, p Pod) {
			p.RunningTask = "task_id"
			require.NoError(t, p.Insert())
			require.NoError(t, p.ClearRunningTask())

			assert.Zero(t, p.RunningTask)
			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.RunningTask)
		},
		"SucceedsWhenNotRunningAnyTask": func(t *testing.T, p Pod) {
			p.RunningTask = ""
			require.NoError(t, p.Insert())
			require.NoError(t, p.ClearRunningTask())

			assert.Zero(t, p.RunningTask)
			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.RunningTask)
		},
		"DoesNotUpdateDBWhenInMemoryPodDoesNotHaveRunningTask": func(t *testing.T, p Pod) {
			p.RunningTask = "task_id"
			require.NoError(t, p.Insert())
			p.RunningTask = ""
			require.NoError(t, p.ClearRunningTask())

			assert.Zero(t, p.RunningTask)
			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.RunningTask)
		},
		"FailsWithNonexistentPod": func(t *testing.T, p Pod) {
			p.RunningTask = "task_id"
			assert.Error(t, p.ClearRunningTask())

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
		"FailsWhenInMemoryPodHasDifferentRunningTaskFromDB": func(t *testing.T, p Pod) {
			p.RunningTask = "task_id"
			require.NoError(t, p.Insert())
			p.RunningTask = "some_other_task_id"
			assert.Error(t, p.ClearRunningTask())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, "task_id", dbPod.RunningTask)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection))

			p := Pod{
				ID:     "pod_id",
				Status: StatusRunning,
			}
			tCase(t, p)
		})
	}
}

func TestSetAgentStartTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod){
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			require.NoError(t, p.Insert())
			require.NoError(t, p.SetAgentStartTime())

			dbPod, err := FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.TimeInfo.AgentStarted)
			assert.Equal(t, p.TimeInfo.AgentStarted, dbPod.TimeInfo.AgentStarted)
		},
		"FailsWithNonexistentPod": func(ctx context.Context, t *testing.T, env *mock.Environment, p Pod) {
			assert.Error(t, p.SetAgentStartTime())

			dbPod, err := FindOneByID(p.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithCancel(ctx)
			defer tcancel()

			require.NoError(t, db.ClearCollections(Collection))

			p := Pod{
				ID: "id",
			}
			tCase(tctx, t, env, p)
		})
	}
}
