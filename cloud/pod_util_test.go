package cloud

import (
	"context"
	"strings"
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

func TestExportECSPod(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault){
		"Succeeds": func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) {
			exported, err := ExportECSPod(p, c, v)
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
					assert.True(t, utility.StringSliceContains(p.Resources.Containers[i].SecretIDs, utility.FromStringPtr(s.ID)))
				}
			}

			ps, err := exportECSPodStatus(p.Status)
			require.NoError(t, err)
			assert.Equal(t, ps, exported.StatusInfo().Status)
		},
		"FailsWithEmptyPod": func(ctx context.Context, t *testing.T, p *pod.Pod, c cocoa.ECSClient, v cocoa.Vault) {
			exported, err := ExportECSPod(&pod.Pod{}, c, v)
			assert.Error(t, err)
			assert.Zero(t, exported)
		},
		"FailsWithInvalidECSClientAndVault": func(ctx context.Context, t *testing.T, p *pod.Pod, _ cocoa.ECSClient, _ cocoa.Vault) {
			exported, err := ExportECSPod(p, nil, nil)
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
		s, err := exportECSPodStatus(pod.StatusStarting)
		require.NoError(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusStarting, s)
	})
	t.Run("SucceedsWithRunningStatus", func(t *testing.T) {
		s, err := exportECSPodStatus(pod.StatusRunning)
		require.NoError(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusRunning, s)
	})
	t.Run("SucceedsWithDecommissionedStatus", func(t *testing.T) {
		s, err := exportECSPodStatus(pod.StatusDecommissioned)
		require.NoError(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusRunning, s)
	})
	t.Run("SucceedsWithTerminatedStatus", func(t *testing.T) {
		s, err := exportECSPodStatus(pod.StatusTerminated)
		require.NoError(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusDeleted, s)
	})
	t.Run("FailsWithInitializingStatus", func(t *testing.T) {
		s, err := exportECSPodStatus(pod.StatusInitializing)
		assert.Error(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusUnknown, s)
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		s, err := exportECSPodStatus("")
		assert.Error(t, err)
		assert.NoError(t, s.Validate())
		assert.Equal(t, cocoa.StatusUnknown, s)
	})
}

func TestExportECSPodResources(t *testing.T) {
	t.Run("SetsNoFields", func(t *testing.T) {
		assert.Zero(t, exportECSPodResources(pod.ResourceInfo{}))
	})
	t.Run("SetsTaskID", func(t *testing.T) {
		id := "task_id"
		r := exportECSPodResources(pod.ResourceInfo{
			ExternalID: id,
		})
		assert.Equal(t, id, utility.FromStringPtr(r.TaskID))
		assert.Zero(t, r.TaskDefinition)
		assert.Zero(t, r.Cluster)
		assert.Zero(t, r.Containers)
	})
	t.Run("SetsCluster", func(t *testing.T) {
		cluster := "cluster"
		r := exportECSPodResources(pod.ResourceInfo{
			Cluster: cluster,
		})
		assert.Equal(t, cluster, utility.FromStringPtr(r.Cluster))
		assert.Zero(t, r.TaskID)
		assert.Zero(t, r.TaskDefinition)
		assert.Zero(t, r.Containers)
	})
	t.Run("SetsTaskDefinitionID", func(t *testing.T) {
		id := "task_def_id"
		r := exportECSPodResources(pod.ResourceInfo{
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
		}
		r := exportECSPodResources(pod.ResourceInfo{
			Containers: []pod.ContainerResourceInfo{c},
		})

		require.Len(t, r.Containers, 1)
		exported := r.Containers[0]
		assert.Equal(t, c.ExternalID, utility.FromStringPtr(exported.ContainerID))
		assert.Equal(t, c.Name, utility.FromStringPtr(exported.Name))
		require.Len(t, exported.Secrets, len(c.SecretIDs))
		for i := range c.SecretIDs {
			assert.True(t, utility.StringSliceContains(c.SecretIDs, utility.FromStringPtr(exported.Secrets[i].ID)))
			assert.True(t, utility.FromBoolPtr(exported.Secrets[i].Owned))
		}
	})
}

func TestImportECSPodResources(t *testing.T) {
	t.Run("SucceedsWithEmptyInput", func(t *testing.T) {
		assert.Zero(t, ImportECSPodResources(*cocoa.NewECSPodResources()))
	})
	t.Run("SucceedsWithTaskID", func(t *testing.T) {
		id := "ecs_task_id"
		res := ImportECSPodResources(*cocoa.NewECSPodResources().SetTaskID(id))
		assert.Equal(t, res.ExternalID, id)
	})
	t.Run("SucceedsWithCluster", func(t *testing.T) {
		cluster := "cluster"
		res := ImportECSPodResources(*cocoa.NewECSPodResources().SetCluster(cluster))
		assert.Equal(t, cluster, res.Cluster)
	})
	t.Run("SucceedsWithTaskDefinition", func(t *testing.T) {
		id := "task_definition_id"
		res := ImportECSPodResources(*cocoa.NewECSPodResources().SetTaskDefinition(
			*cocoa.NewECSTaskDefinition().
				SetID(id)))
		assert.Equal(t, res.DefinitionID, id)
	})
	t.Run("SucceedsWithContainers", func(t *testing.T) {
		c := *cocoa.NewECSContainerResources().
			SetContainerID("container_id").
			SetName("container_name").
			AddSecrets(*cocoa.NewContainerSecret().SetID("secret_id"))
		res := ImportECSPodResources(*cocoa.NewECSPodResources().AddContainers(c))
		require.Len(t, res.Containers, 1)
		cRes := res.Containers[0]
		assert.Equal(t, utility.FromStringPtr(c.ContainerID), cRes.ExternalID)
		assert.Equal(t, utility.FromStringPtr(c.Name), cRes.Name)
		require.Len(t, cRes.SecretIDs, len(c.Secrets))
		for i := range cRes.SecretIDs {
			assert.Equal(t, utility.FromStringPtr(c.Secrets[i].ID), cRes.SecretIDs[i])
		}
	})
}

func TestExportECSPodStatusInfo(t *testing.T) {
	t.Run("SucceedsWithValidStatus", func(t *testing.T) {
		p := pod.Pod{
			Status: pod.StatusRunning,
		}
		ps, err := exportECSPodStatus(p.Status)
		require.NoError(t, err)
		exported, err := exportECSPodStatusInfo(&p)
		require.NoError(t, err)
		assert.NoError(t, ps.Validate())
		assert.Equal(t, ps, exported.Status)
	})
	t.Run("FailsWithInvalidStatus", func(t *testing.T) {
		p := pod.Pod{
			Status: pod.StatusInitializing,
		}
		exported, err := exportECSPodStatusInfo(&p)
		assert.Error(t, err)
		assert.Zero(t, exported)
	})
	t.Run("FailsWithEmptyStatus", func(t *testing.T) {
		p := pod.Pod{
			Status: "",
		}
		exported, err := exportECSPodStatusInfo(&p)
		assert.Error(t, err)
		assert.Zero(t, exported)
	})
}

func TestExportECSContainerStatusInfo(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		ci := pod.ContainerResourceInfo{
			ExternalID: "container_id",
			Name:       "container_name",
		}
		exported := exportECSContainerStatusInfo(ci)
		assert.NoError(t, exported.Validate())
		assert.Equal(t, ci.ExternalID, utility.FromStringPtr(exported.ContainerID))
		assert.Equal(t, ci.Name, utility.FromStringPtr(exported.Name))
		assert.Equal(t, cocoa.StatusUnknown, exported.Status)
	})
}

func TestExportECSPodCreationOptions(t *testing.T) {
	validPod := func() *pod.Pod {
		return &pod.Pod{
			TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
				Image:      "image",
				MemoryMB:   128,
				CPU:        128,
				OS:         pod.OSLinux,
				Arch:       pod.ArchAMD64,
				WorkingDir: "/root",
				EnvVars: map[string]string{
					"ENV_VAR": "value",
				},
				EnvSecrets: map[string]pod.Secret{
					"SECRET_ENV_VAR": {
						Name:       "name0",
						ExternalID: "external_id",
						Value:      "value0",
						Exists:     utility.TruePtr(),
						Owned:      utility.FalsePtr(),
					},
					"SHARED_SECRET_ENV_VAR": {
						Name:   "name1",
						Value:  "value1",
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
					"UNNAMED_SECRET_ENV_VAR": {
						Value:  "value2",
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
				},
			},
		}
	}

	validSettings := func() *evergreen.Settings {
		return &evergreen.Settings{
			Providers: evergreen.CloudProviders{
				AWS: evergreen.AWSConfig{
					Pod: evergreen.AWSPodConfig{
						ECS: evergreen.ECSConfig{
							TaskRole:             "task_role",
							TaskDefinitionPrefix: "task_definition_prefix",
							ExecutionRole:        "execution_role",
							AWSVPC: evergreen.AWSVPCConfig{
								Subnets:        []string{"subnet-12345"},
								SecurityGroups: []string{"sg-12345"},
							},
							Clusters: []evergreen.ECSClusterConfig{
								{
									Platform: "linux",
									Name:     "cluster",
								},
							},
						},
						SecretsManager: evergreen.SecretsManagerConfig{
							SecretPrefix: "secret_prefix",
						},
					},
				},
			},
		}
	}

	t.Run("Succeeds", func(t *testing.T) {
		settings := validSettings()
		p := validPod()
		opts, err := ExportECSPodCreationOptions(settings, p)
		require.NoError(t, err)
		require.NotZero(t, opts)
		require.Equal(t, settings.Providers.AWS.Pod.ECS.TaskRole, utility.FromStringPtr(opts.TaskRole))
		require.Equal(t, settings.Providers.AWS.Pod.ECS.ExecutionRole, utility.FromStringPtr(opts.ExecutionRole))
		require.NotZero(t, opts.NetworkMode)
		assert.Equal(t, cocoa.NetworkModeAWSVPC, *opts.NetworkMode)

		require.NotZero(t, opts.ExecutionOpts)
		require.Equal(t, settings.Providers.AWS.Pod.ECS.Clusters[0].Name, utility.FromStringPtr(opts.ExecutionOpts.Cluster))
		require.NotZero(t, opts.ExecutionOpts.AWSVPCOpts)
		assert.Equal(t, settings.Providers.AWS.Pod.ECS.AWSVPC.Subnets, opts.ExecutionOpts.AWSVPCOpts.Subnets)
		assert.Equal(t, settings.Providers.AWS.Pod.ECS.AWSVPC.SecurityGroups, opts.ExecutionOpts.AWSVPCOpts.SecurityGroups)
		require.NotZero(t, opts.ExecutionOpts.PlacementOpts)
		require.Len(t, opts.ExecutionOpts.PlacementOpts.InstanceFilters, 1)
		assert.Equal(t, "attribute:ecs.cpu-architecture == x86_64", opts.ExecutionOpts.PlacementOpts.InstanceFilters[0])

		assert.True(t, strings.HasPrefix(utility.FromStringPtr(opts.Name), settings.Providers.AWS.Pod.ECS.TaskDefinitionPrefix))
		assert.Contains(t, utility.FromStringPtr(opts.Name), p.ID)

		require.Len(t, opts.ContainerDefinitions, 1)
		cDef := opts.ContainerDefinitions[0]
		require.Equal(t, p.TaskContainerCreationOpts.Image, utility.FromStringPtr(cDef.Image))
		require.Equal(t, p.TaskContainerCreationOpts.MemoryMB, utility.FromIntPtr(cDef.MemoryMB))
		require.Equal(t, p.TaskContainerCreationOpts.CPU, utility.FromIntPtr(cDef.CPU))
		require.Equal(t, p.TaskContainerCreationOpts.WorkingDir, utility.FromStringPtr(cDef.WorkingDir))
		require.Len(t, cDef.PortMappings, 1)
		assert.Equal(t, agentPort, utility.FromIntPtr(cDef.PortMappings[0].ContainerPort))
		require.Len(t, cDef.EnvVars, 4)
		for _, envVar := range cDef.EnvVars {
			envVarName := utility.FromStringPtr(envVar.Name)
			switch envVarName {
			case "ENV_VAR":
				assert.Equal(t, p.TaskContainerCreationOpts.EnvVars[utility.FromStringPtr(envVar.Name)], utility.FromStringPtr(envVar.Value))
			case "SECRET_ENV_VAR":
				s := p.TaskContainerCreationOpts.EnvSecrets[utility.FromStringPtr(envVar.Name)]
				assert.Zero(t, envVar.SecretOpts.NewValue)
				assert.Zero(t, envVar.SecretOpts.Name)
				secretName := utility.FromStringPtr(envVar.SecretOpts.ID)
				assert.Equal(t, s.ExternalID, secretName)
				assert.False(t, utility.FromBoolPtr(envVar.SecretOpts.Owned))
			case "SHARED_SECRET_ENV_VAR":
				s := p.TaskContainerCreationOpts.EnvSecrets[utility.FromStringPtr(envVar.Name)]
				assert.Equal(t, s.Value, utility.FromStringPtr(envVar.SecretOpts.NewValue))
				assert.Zero(t, envVar.SecretOpts.ID)
				secretName := utility.FromStringPtr(envVar.SecretOpts.Name)
				assert.True(t, strings.HasPrefix(secretName, settings.Providers.AWS.Pod.SecretsManager.SecretPrefix))
				assert.Contains(t, secretName, p.ID)
				assert.Contains(t, secretName, s.Name)
				assert.True(t, utility.FromBoolPtr(envVar.SecretOpts.Owned))
			case "UNNAMED_SECRET_ENV_VAR":
				s := p.TaskContainerCreationOpts.EnvSecrets[utility.FromStringPtr(envVar.Name)]
				assert.Equal(t, s.Value, utility.FromStringPtr(envVar.SecretOpts.NewValue))
				assert.Zero(t, envVar.SecretOpts.ID)
				secretName := utility.FromStringPtr(envVar.SecretOpts.Name)
				assert.True(t, strings.HasPrefix(secretName, settings.Providers.AWS.Pod.SecretsManager.SecretPrefix))
				assert.Contains(t, secretName, p.ID)
				assert.Contains(t, secretName, envVarName)
				assert.True(t, utility.FromBoolPtr(envVar.SecretOpts.Owned))
			default:
				require.FailNow(t, "unexpected environment variable '%s'", envVarName)
			}
		}
	})
	t.Run("SucceedsWithRepositoryCredentials", func(t *testing.T) {
		settings := validSettings()
		p := validPod()
		p.TaskContainerCreationOpts.RepoUsername = "username"
		p.TaskContainerCreationOpts.RepoPassword = "password"
		opts, err := ExportECSPodCreationOptions(settings, p)
		require.NoError(t, err)
		require.NotZero(t, opts)

		require.Len(t, opts.ContainerDefinitions, 1)
		cDef := opts.ContainerDefinitions[0]
		assert.True(t, strings.HasPrefix(utility.FromStringPtr(cDef.RepoCreds.Name), settings.Providers.AWS.Pod.SecretsManager.SecretPrefix))
		assert.Contains(t, utility.FromStringPtr(cDef.RepoCreds.Name), p.ID)
		assert.Equal(t, utility.FromStringPtr(cDef.RepoCreds.NewCreds.Username), p.TaskContainerCreationOpts.RepoUsername)
		assert.Equal(t, utility.FromStringPtr(cDef.RepoCreds.NewCreds.Password), p.TaskContainerCreationOpts.RepoPassword)
	})
	t.Run("OnlyUsesAWSVPCWhenAWSVPCSettingsAreGiven", func(t *testing.T) {
		settings := validSettings()
		settings.Providers.AWS.Pod.ECS.AWSVPC = evergreen.AWSVPCConfig{}
		p := validPod()
		opts, err := ExportECSPodCreationOptions(settings, p)
		require.NoError(t, err)
		assert.NoError(t, opts.Validate())
		assert.Zero(t, opts.NetworkMode)
	})
	t.Run("FailsWithNoECSConfig", func(t *testing.T) {
		settings := validSettings()
		settings.Providers.AWS.Pod.ECS = evergreen.ECSConfig{}
		opts, err := ExportECSPodCreationOptions(settings, validPod())
		require.NotZero(t, err)
		assert.Zero(t, opts)
	})
	t.Run("FailsWithNoClusters", func(t *testing.T) {
		settings := validSettings()
		settings.Providers.AWS.Pod.ECS.Clusters = nil
		opts, err := ExportECSPodCreationOptions(settings, &pod.Pod{})
		assert.Error(t, err)
		assert.Zero(t, opts)
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
