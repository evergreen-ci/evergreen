package cloud

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMakeECSClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeECSClient(validPodClientSettings())
		assert.NoError(t, err)
		assert.NotZero(t, c)
		assert.NoError(t, c.Close(ctx))
	})
	t.Run("FailsWithoutRequiredSettings", func(t *testing.T) {
		c, err := MakeECSClient(&evergreen.Settings{})
		assert.Error(t, err)
		assert.Zero(t, c)
	})
}

func TestMakeSecretsManagerClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(validPodClientSettings())
		assert.NoError(t, err)
		assert.NotZero(t, c)
		assert.NoError(t, c.Close(ctx))
	})
	t.Run("FailsWithoutRequiredSettings", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(&evergreen.Settings{})
		assert.Error(t, err)
		assert.Zero(t, c)
	})
}

func TestMakeSecretsManagerVault(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(validPodClientSettings())
		require.NoError(t, err)
		assert.NotZero(t, MakeSecretsManagerVault(c))
	})
}

func TestMakeECSPodCreator(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeECSPodCreator(&mock.ECSClient{}, &mock.Vault{})
		require.NoError(t, err)
		assert.NotZero(t, c)
	})
	t.Run("FailsWithoutRequiredClient", func(t *testing.T) {
		c, err := MakeECSPodCreator(nil, &mock.Vault{})
		require.Error(t, err)
		assert.Zero(t, c)
	})
}

func TestExportPod(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault){
		"Succeeds": func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) {
			exported, err := ExportPod(p, c, v)
			require.NoError(t, err)

			resources := exported.Resources()
			assert.Equal(t, p.Resources.ExternalID, utility.FromStringPtr(resources.TaskID))
			require.NotZero(t, resources.TaskDefinition)
			assert.Equal(t, p.Resources.DefinitionID, utility.FromStringPtr(resources.TaskDefinition.ID))
			assert.True(t, utility.FromBoolPtr(resources.TaskDefinition.Owned))
			assert.Equal(t, p.Resources.Cluster, utility.FromStringPtr(resources.Cluster))
			require.Len(t, resources.Containers, len(p.Resources.Containers))
			for i := range p.Resources.Containers {
				assert.Equal(t, p.Resources.Containers[i].ExternalID, utility.FromStringPtr(resources.Containers[i].ContainerID))
				assert.Equal(t, p.Resources.Containers[i].Name, utility.FromStringPtr(resources.Containers[i].Name))
				require.Len(t, resources.Containers[i].Secrets, len(p.Resources.Containers[i].SecretIDs))
				for _, s := range resources.Containers[i].Secrets {
					assert.True(t, utility.StringSliceContains(p.Resources.Containers[i].SecretIDs, utility.FromStringPtr(s.Name)))
				}
			}

			stat := exported.StatusInfo()
			ps, err := ExportECSPodStatus(p.Status)
			require.NoError(t, err)
			assert.Equal(t, ps, stat.Status)
		},
		"FailsWithEmptyPod": func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) {
			exported, err := ExportPod(&pod.Pod{}, c, v)
			assert.Error(t, err)
			assert.Zero(t, exported)
		},
		"FailsWithInvalidECSClientAndVault": func(ctx context.Context, t *testing.T, p *pod.Pod, _ cocoa.ECSClient, _ cocoa.Vault) {
			exported, err := ExportPod(p, nil, nil)
			assert.Error(t, err)
			assert.Zero(t, exported)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			p := pod.Pod{
				ID:     "id",
				Status: pod.StatusRunning,
				Resources: pod.ResourceInfo{
					ExternalID:   "task_id",
					DefinitionID: "task_def_id",
					Cluster:      "cluster",
					Containers: []pod.ContainerResourceInfo{
						{
							ExternalID: "container_id",
							Name:       "container_name",
							SecretIDs:  []string{"secret"},
							Status:     pod.ContainerStatusRunning,
						},
					},
				},
			}
			ecsClient, err := MakeECSClient(validPodClientSettings())
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, ecsClient.Close(ctx))
			}()
			smClient, err := MakeSecretsManagerClient(validPodClientSettings())
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, smClient.Close(ctx))
			}()
			vault := MakeSecretsManagerVault(smClient)

			tCase(ctx, t, &p, ecsClient, vault)
		})
	}
}

func TestExportECSPodStatus(t *testing.T) {
	t.Run("SucceedsWithStartingStatus", func(t *testing.T) {
		s, err := ExportECSPodStatus(pod.StatusStarting)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusStarting, s)
	})
	t.Run("SucceedsWithRunningStatus", func(t *testing.T) {
		s, err := ExportECSPodStatus(pod.StatusRunning)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusRunning, s)
	})
	t.Run("SucceedsWithTerminatedStatus", func(t *testing.T) {
		s, err := ExportECSPodStatus(pod.StatusTerminated)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusDeleted, s)
	})
	t.Run("FailsWithInitializingStatus", func(t *testing.T) {
		s, err := ExportECSPodStatus(pod.StatusInitializing)
		assert.Error(t, err)
		assert.Equal(t, cocoa.StatusUnknown, s)
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		s, err := ExportECSPodStatus("")
		assert.Error(t, err)
		assert.Equal(t, cocoa.StatusUnknown, s)
	})
}

func TestExportECSContainerStatus(t *testing.T) {
	t.Run("SucceedsWithStartingStatus", func(t *testing.T) {
		s, err := ExportECSContainerStatus(pod.ContainerStatusStarting)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusStarting, s)
	})
	t.Run("SucceedsWithRunningStatus", func(t *testing.T) {
		s, err := ExportECSContainerStatus(pod.ContainerStatusRunning)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusRunning, s)
	})
	t.Run("SucceedsWithStoppedStatus", func(t *testing.T) {
		s, err := ExportECSContainerStatus(pod.ContainerStatusStopped)
		require.NoError(t, err)
		assert.Equal(t, cocoa.StatusStopped, s)
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		s, err := ExportECSContainerStatus("")
		assert.Error(t, err)
		assert.Equal(t, cocoa.StatusUnknown, s)
	})
}

func TestImportECSContainerStatus(t *testing.T) {
	t.Run("SucceedsWithStartingStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus(cocoa.StatusStarting)
		require.NoError(t, err)
		assert.Equal(t, pod.ContainerStatusStarting, s)
	})
	t.Run("SucceedsWithRunningStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus(cocoa.StatusRunning)
		require.NoError(t, err)
		assert.Equal(t, pod.ContainerStatusRunning, s)
	})
	t.Run("SucceedsWithStoppedStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus(cocoa.StatusStopped)
		require.NoError(t, err)
		assert.Equal(t, pod.ContainerStatusStopped, s)
	})
	t.Run("SucceedsWithDeletedStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus(cocoa.StatusDeleted)
		require.NoError(t, err)
		assert.Equal(t, pod.ContainerStatusStopped, s)
	})
	t.Run("FailsWithUnknownStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus(cocoa.StatusUnknown)
		assert.Error(t, err)
		assert.Zero(t, s)
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		s, err := ImportECSContainerStatus("")
		assert.Error(t, err)
		assert.Zero(t, s)
	})
}

func TestExportPodResources(t *testing.T) {
	t.Run("SetsNoFields", func(t *testing.T) {
		assert.Zero(t, ExportPodResources(pod.ResourceInfo{}))
	})
	t.Run("SetsTaskID", func(t *testing.T) {
		id := "task_id"
		r := ExportPodResources(pod.ResourceInfo{
			ExternalID: id,
		})
		assert.Equal(t, id, utility.FromStringPtr(r.TaskID))
		assert.Zero(t, r.TaskDefinition)
		assert.Zero(t, r.Cluster)
		assert.Zero(t, r.Containers)
	})
	t.Run("SetsCluster", func(t *testing.T) {
		cluster := "cluster"
		r := ExportPodResources(pod.ResourceInfo{
			Cluster: cluster,
		})
		assert.Equal(t, cluster, utility.FromStringPtr(r.Cluster))
		assert.Zero(t, r.TaskID)
		assert.Zero(t, r.TaskDefinition)
		assert.Zero(t, r.Containers)
	})
	t.Run("SetsTaskDefinitionID", func(t *testing.T) {
		id := "task_def_id"
		r := ExportPodResources(pod.ResourceInfo{
			DefinitionID: id,
		})
		require.NotZero(t, r.TaskDefinition)
		assert.Equal(t, id, utility.FromStringPtr(r.TaskDefinition.ID))
		assert.True(t, utility.FromBoolPtr(r.TaskDefinition.Owned))
		assert.Zero(t, r.TaskID)
		assert.Zero(t, r.Cluster)
		assert.Zero(t, r.Containers)
	})
	t.Run("SetsContainers", func(t *testing.T) {
		c := pod.ContainerResourceInfo{
			ExternalID: "container_id",
			Name:       "container_name",
			SecretIDs:  []string{"secret0", "secret1"},
			Status:     pod.ContainerStatusRunning,
		}
		r := ExportPodResources(pod.ResourceInfo{
			Containers: []pod.ContainerResourceInfo{c},
		})

		require.Len(t, r.Containers, 1)
		exported := r.Containers[0]
		assert.Equal(t, c.ExternalID, utility.FromStringPtr(exported.ContainerID))
		assert.Equal(t, c.Name, utility.FromStringPtr(exported.Name))
		require.Len(t, exported.Secrets, len(c.SecretIDs))
		for i := range c.SecretIDs {
			assert.True(t, utility.StringSliceContains(c.SecretIDs, utility.FromStringPtr(exported.Secrets[i].Name)))
			assert.True(t, utility.FromBoolPtr(exported.Secrets[i].Owned))
		}
	})
}

func TestExportECSPodStatusInfo(t *testing.T) {
	t.Run("SucceedsWithoutContainerStatusInfo", func(t *testing.T) {
		p := pod.Pod{
			Status: pod.StatusRunning,
		}
		exported, err := ExportECSPodStatusInfo(&p)
		require.NoError(t, err)
		ps, err := ExportECSPodStatus(p.Status)
		require.NoError(t, err)
		assert.Equal(t, ps, exported.Status)
		assert.Empty(t, exported.Containers)
	})
	t.Run("SucceedsWithContainerStatusInfo", func(t *testing.T) {
		ci := pod.ContainerResourceInfo{
			ExternalID: "container_id",
			Name:       "container_name",
			Status:     pod.ContainerStatusRunning,
		}
		p := pod.Pod{
			Status: pod.StatusRunning,
			Resources: pod.ResourceInfo{
				Containers: []pod.ContainerResourceInfo{ci},
			},
		}
		exported, err := ExportECSPodStatusInfo(&p)
		require.NoError(t, err)
		ps, err := ExportECSPodStatus(p.Status)
		require.NoError(t, err)
		assert.Equal(t, ps, exported.Status)
		cs, err := ExportECSContainerStatus(ci.Status)
		require.Len(t, exported.Containers, 1)
		exportedContainer := exported.Containers[0]
		require.NoError(t, err)
		assert.Equal(t, cs, exportedContainer.Status)
		assert.Equal(t, ci.ExternalID, utility.FromStringPtr(exportedContainer.ContainerID))
		assert.Equal(t, ci.Name, utility.FromStringPtr(exportedContainer.Name))
	})
	t.Run("FailsWithInvalidContainerStatus", func(t *testing.T) {
		ci := pod.ContainerResourceInfo{}
		p := pod.Pod{
			Status: pod.StatusRunning,
			Resources: pod.ResourceInfo{
				Containers: []pod.ContainerResourceInfo{ci},
			},
		}
		exported, err := ExportECSPodStatusInfo(&p)
		assert.Error(t, err)
		assert.Zero(t, exported)
	})
	t.Run("FailsWithInvalidPodStatus", func(t *testing.T) {
		p := pod.Pod{
			Status: "",
		}
		exported, err := ExportECSPodStatusInfo(&p)
		assert.Error(t, err)
		assert.Zero(t, exported)
	})
}

func TestExportECSContainerStatusInfo(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		ci := pod.ContainerResourceInfo{
			ExternalID: "container_id",
			Name:       "container_name",
			Status:     pod.ContainerStatusRunning,
		}
		exported, err := ExportECSContainerStatusInfo(ci)
		require.NoError(t, err)
		cs, err := ExportECSContainerStatus(ci.Status)
		require.NoError(t, err)
		assert.Equal(t, cs, exported.Status)
		assert.Equal(t, ci.ExternalID, utility.FromStringPtr(exported.ContainerID))
		assert.Equal(t, ci.Name, utility.FromStringPtr(exported.Name))
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		ci := pod.ContainerResourceInfo{
			ExternalID: "container_id",
			Name:       "container_name",
			Status:     "",
		}
		exported, err := ExportECSContainerStatusInfo(ci)
		assert.Error(t, err)
		assert.Zero(t, exported)
	})
}

func TestExportPodCreationOptions(t *testing.T) {
	t.Run("FailsWithNoECSConfig", func(t *testing.T) {
		opts, err := ExportPodCreationOptions(evergreen.ECSConfig{}, pod.TaskContainerCreationOptions{})
		require.NotZero(t, err)
		require.Zero(t, opts)
	})
	t.Run("FailsWithNoClusterName", func(t *testing.T) {
		opts, err := ExportPodCreationOptions(
			evergreen.ECSConfig{
				TaskRole:      "role",
				ExecutionRole: "role",
				Clusters: []evergreen.ECSClusterConfig{
					{
						Platform: "linux",
					},
				},
			}, pod.TaskContainerCreationOptions{})
		require.NotZero(t, err)
		require.Zero(t, opts)
	})
	t.Run("Succeeds", func(t *testing.T) {
		opts, err := ExportPodCreationOptions(
			evergreen.ECSConfig{
				TaskRole:      "task_role",
				ExecutionRole: "execution_role",
				Clusters: []evergreen.ECSClusterConfig{
					{
						Name: "cluster",
					},
				},
			}, pod.TaskContainerCreationOptions{
				Image:    "image",
				MemoryMB: 128,
				CPU:      128,
				EnvVars: map[string]string{
					"name": "value",
				},
				EnvSecrets: map[string]string{
					"s1": "secret",
				},
			})
		require.Zero(t, err)
		require.NotZero(t, opts)
		require.Equal(t, "task_role", utility.FromStringPtr(opts.TaskRole))
		require.Equal(t, "execution_role", utility.FromStringPtr(opts.ExecutionRole))

		require.NotZero(t, opts.ExecutionOpts)
		require.Equal(t, "cluster", utility.FromStringPtr(opts.ExecutionOpts.Cluster))

		require.NotZero(t, opts.ContainerDefinitions)
		require.Len(t, opts.ContainerDefinitions, 1)
		require.Equal(t, "image", utility.FromStringPtr(opts.ContainerDefinitions[0].Image))
		require.Equal(t, 128, utility.FromIntPtr(opts.ContainerDefinitions[0].MemoryMB))
		require.Equal(t, 128, utility.FromIntPtr(opts.ContainerDefinitions[0].CPU))
		require.Len(t, opts.ContainerDefinitions[0].EnvVars, 2)
		for _, envVar := range opts.ContainerDefinitions[0].EnvVars {
			if envVar.SecretOpts != nil {
				require.Equal(t, "s1", utility.FromStringPtr(envVar.SecretOpts.Name))
				require.Equal(t, "secret", utility.FromStringPtr(envVar.SecretOpts.Value))
				require.Equal(t, false, utility.FromBoolPtr(envVar.SecretOpts.Exists))
				require.Equal(t, true, utility.FromBoolPtr(envVar.SecretOpts.Owned))
			} else {
				require.Equal(t, "name", utility.FromStringPtr(envVar.Name))
				require.Equal(t, "value", utility.FromStringPtr(envVar.Value))
			}
		}
	})
}

func validPodClientSettings() *evergreen.Settings {
	return &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				Pod: evergreen.AWSPodConfig{
					Region: "region",
					Role:   "role",
				},
			},
		},
	}
}
