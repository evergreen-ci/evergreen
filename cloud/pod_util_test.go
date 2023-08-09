package cloud

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	ecsTypes "github.com/aws/aws-sdk-go-v2/service/ecs/types"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/cocoa/secret"
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
		c, err := MakeECSClient(ctx, validPodClientSettings())
		assert.NoError(t, err)
		assert.NotZero(t, c)
		assert.NoError(t, c.Close(ctx))
	})
	t.Run("FailsWithoutRequiredSettings", func(t *testing.T) {
		c, err := MakeECSClient(ctx, &evergreen.Settings{})
		assert.Error(t, err)
		assert.Zero(t, c)
	})
}

func TestMakeSecretsManagerClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(ctx, validPodClientSettings())
		assert.NoError(t, err)
		assert.NotZero(t, c)
		assert.NoError(t, c.Close(ctx))
	})
	t.Run("FailsWithoutRequiredSettings", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(ctx, &evergreen.Settings{})
		assert.Error(t, err)
		assert.Zero(t, c)
	})
}

func TestMakeSecretsManagerVault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeSecretsManagerClient(ctx, validPodClientSettings())
		require.NoError(t, err)
		v, err := MakeSecretsManagerVault(c)
		assert.NoError(t, err)
		assert.NotZero(t, v)
	})
}

func TestMakeECSPodCreator(t *testing.T) {
	t.Run("Succeeds", func(t *testing.T) {
		c, err := MakeECSPodCreator(&cocoaMock.ECSClient{}, &cocoaMock.Vault{})
		require.NoError(t, err)
		assert.NotZero(t, c)
	})
	t.Run("FailsWithoutRequiredClient", func(t *testing.T) {
		c, err := MakeECSPodCreator(nil, &cocoaMock.Vault{})
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
			assert.False(t, utility.FromBoolPtr(resources.TaskDefinition.Owned))
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
			ecsClient, err := MakeECSClient(ctx, validPodClientSettings())
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, ecsClient.Close(ctx))
			}()
			smClient, err := MakeSecretsManagerClient(ctx, validPodClientSettings())
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, smClient.Close(ctx))
			}()
			vault, err := MakeSecretsManagerVault(smClient)
			require.NoError(t, err)

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
		assert.False(t, utility.FromBoolPtr(r.TaskDefinition.Owned))
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
			assert.False(t, utility.FromBoolPtr(exported.Secrets[i].Owned))
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

func TestExportECSPodDefinitionOptions(t *testing.T) {
	validContainerOpts := func() pod.TaskContainerCreationOptions {
		return pod.TaskContainerCreationOptions{
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
					ExternalID: "external_id",
					Value:      "value0",
				},
			},
		}
	}

	validSettings := func() evergreen.Settings {
		return evergreen.Settings{
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
							LogRegion:       "us-east-1",
							LogGroup:        "log_group",
							LogStreamPrefix: "log_stream_prefix",
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
		containerOpts := validContainerOpts()
		podDefOpts, err := ExportECSPodDefinitionOptions(&settings, containerOpts)
		require.NoError(t, err)
		require.NotZero(t, podDefOpts)
		require.Equal(t, settings.Providers.AWS.Pod.ECS.TaskRole, utility.FromStringPtr(podDefOpts.TaskRole))
		require.Equal(t, settings.Providers.AWS.Pod.ECS.ExecutionRole, utility.FromStringPtr(podDefOpts.ExecutionRole))
		require.NotZero(t, podDefOpts.NetworkMode)
		assert.Equal(t, cocoa.NetworkModeAWSVPC, *podDefOpts.NetworkMode)

		assert.True(t, strings.HasPrefix(utility.FromStringPtr(podDefOpts.Name), settings.Providers.AWS.Pod.ECS.TaskDefinitionPrefix))
		assert.Contains(t, utility.FromStringPtr(podDefOpts.Name), containerOpts.Hash())
		require.Equal(t, containerOpts.CPU, utility.FromIntPtr(podDefOpts.CPU))
		require.Equal(t, containerOpts.MemoryMB, utility.FromIntPtr(podDefOpts.MemoryMB))

		require.Len(t, podDefOpts.ContainerDefinitions, 1)
		cDef := podDefOpts.ContainerDefinitions[0]
		require.Equal(t, containerOpts.Image, utility.FromStringPtr(cDef.Image))
		require.Equal(t, containerOpts.MemoryMB, utility.FromIntPtr(cDef.MemoryMB))
		require.Equal(t, containerOpts.CPU, utility.FromIntPtr(cDef.CPU))
		require.Equal(t, containerOpts.WorkingDir, utility.FromStringPtr(cDef.WorkingDir))
		require.Len(t, cDef.PortMappings, 1)
		assert.Equal(t, agentPort, utility.FromIntPtr(cDef.PortMappings[0].ContainerPort))
		assert.Equal(t, ecsTypes.LogDriverAwslogs, utility.FromStringPtr(cDef.LogConfiguration.LogDriver))
		assert.Equal(t, "us-east-1", cDef.LogConfiguration.Options[awsLogsRegion])
		assert.Equal(t, "log_group", cDef.LogConfiguration.Options[awsLogsGroup])

		require.Len(t, cDef.EnvVars, 1, "container definition should contain just the secret environment variable, not the plaintext one")
		expectedSecret := containerOpts.EnvSecrets[utility.FromStringPtr(cDef.EnvVars[0].Name)]
		assert.Equal(t, expectedSecret.ExternalID, utility.FromStringPtr(cDef.EnvVars[0].SecretOpts.ID))
		assert.False(t, utility.FromBoolPtr(cDef.EnvVars[0].SecretOpts.Owned))
	})
	t.Run("SucceedsWithRepositoryCredentials", func(t *testing.T) {
		settings := validSettings()
		containerOpts := validContainerOpts()
		containerOpts.RepoCredsExternalID = "repo_credss_external_id"
		podDefOpts, err := ExportECSPodDefinitionOptions(&settings, containerOpts)
		require.NoError(t, err)
		require.NotZero(t, containerOpts)

		require.Len(t, podDefOpts.ContainerDefinitions, 1)
		cDef := podDefOpts.ContainerDefinitions[0]
		require.NotZero(t, cDef.RepoCreds)
		assert.Equal(t, utility.FromStringPtr(cDef.RepoCreds.ID), containerOpts.RepoCredsExternalID)
	})
	t.Run("DefaultsToBridgeNetworkingWhenAWSVPCSettingsAreUnset", func(t *testing.T) {
		settings := validSettings()
		settings.Providers.AWS.Pod.ECS.AWSVPC = evergreen.AWSVPCConfig{}
		podDefOpts, err := ExportECSPodDefinitionOptions(&settings, validContainerOpts())
		require.NoError(t, err)
		assert.NoError(t, podDefOpts.Validate())
		assert.Zero(t, podDefOpts.NetworkMode)
	})
}

func TestExportECSPodExecutionOptions(t *testing.T) {
	getECSConfig := func() evergreen.ECSConfig {
		return evergreen.ECSConfig{
			TaskRole:             "task_role",
			TaskDefinitionPrefix: "task_definition_prefix",
			ExecutionRole:        "execution_role",
			AWSVPC: evergreen.AWSVPCConfig{
				Subnets:        []string{"subnet-12345"},
				SecurityGroups: []string{"sg-12345"},
			},
			Clusters: []evergreen.ECSClusterConfig{
				{
					OS:   evergreen.ECSOSLinux,
					Name: "linux_cluster",
				},
				{
					OS:   evergreen.ECSOSWindows,
					Name: "windows_cluster",
				},
			},
			CapacityProviders: []evergreen.ECSCapacityProvider{
				{
					Name: "linux_capacity_provider",
					OS:   evergreen.ECSOSLinux,
					Arch: evergreen.ECSArchAMD64,
				},
				{
					Name:           "windows_capacity_provider",
					OS:             evergreen.ECSOSWindows,
					Arch:           evergreen.ECSArchAMD64,
					WindowsVersion: evergreen.ECSWindowsServer2022,
				},
			},
			AllowedImages: []string{"it_is_allowed"},
		}
	}
	getContainerOpts := func() pod.TaskContainerCreationOptions {
		return pod.TaskContainerCreationOptions{
			OS:      pod.OSLinux,
			Arch:    pod.ArchAMD64,
			EnvVars: map[string]string{"ENV_VAR": "value"},
		}
	}

	t.Run("SpecifiesLinuxClusterAndCapacityProviderWithLinuxPod", func(t *testing.T) {
		ecsConf := getECSConfig()
		containerOpts := getContainerOpts()
		execOpts, err := ExportECSPodExecutionOptions(ecsConf, containerOpts)
		require.NoError(t, err)
		require.NotZero(t, execOpts)

		require.Equal(t, ecsConf.Clusters[0].Name, utility.FromStringPtr(execOpts.Cluster))
		assert.Equal(t, ecsConf.CapacityProviders[0].Name, utility.FromStringPtr(execOpts.CapacityProvider))
		require.NotZero(t, execOpts.AWSVPCOpts)
		assert.Equal(t, ecsConf.AWSVPC.Subnets, execOpts.AWSVPCOpts.Subnets)
		assert.Equal(t, ecsConf.AWSVPC.SecurityGroups, execOpts.AWSVPCOpts.SecurityGroups)
		require.NotZero(t, execOpts.OverrideOpts)
		require.Len(t, execOpts.OverrideOpts.ContainerDefinitions, 1, "should override the pod definition's container definition")
		overrideContainerDef := execOpts.OverrideOpts.ContainerDefinitions[0]
		require.Len(t, overrideContainerDef.EnvVars, 1, "should override the container definition's plaintext environment variables")
		overrideEnvVar := overrideContainerDef.EnvVars[0]
		assert.Equal(t, "ENV_VAR", utility.FromStringPtr(overrideEnvVar.Name), "should override environment variable")
		assert.Equal(t, containerOpts.EnvVars["ENV_VAR"], utility.FromStringPtr(overrideEnvVar.Value), "should override plaintext environment variable's value")
	})
	t.Run("OmitsAWSVPCOptionsWhenUnset", func(t *testing.T) {
		ecsConf := getECSConfig()
		ecsConf.AWSVPC = evergreen.AWSVPCConfig{}
		containerOpts := getContainerOpts()
		execOpts, err := ExportECSPodExecutionOptions(ecsConf, containerOpts)
		require.NoError(t, err)
		require.NotZero(t, execOpts)

		assert.Zero(t, execOpts.AWSVPCOpts)
		require.Equal(t, ecsConf.Clusters[0].Name, utility.FromStringPtr(execOpts.Cluster))
		assert.Equal(t, ecsConf.CapacityProviders[0].Name, utility.FromStringPtr(execOpts.CapacityProvider))
	})
	t.Run("SpecifiesWindowsClusterAndCapacityProviderWithWindowsPod", func(t *testing.T) {
		ecsConf := getECSConfig()
		containerOpts := getContainerOpts()
		containerOpts.OS = pod.OSWindows
		containerOpts.Arch = pod.ArchAMD64
		containerOpts.WindowsVersion = pod.WindowsVersionServer2022

		execOpts, err := ExportECSPodExecutionOptions(ecsConf, containerOpts)
		require.NoError(t, err)
		require.NotZero(t, execOpts)
		require.Equal(t, ecsConf.Clusters[1].Name, utility.FromStringPtr(execOpts.Cluster))
		assert.Equal(t, ecsConf.CapacityProviders[1].Name, utility.FromStringPtr(execOpts.CapacityProvider))
	})
	t.Run("FailsWithNoECSConfig", func(t *testing.T) {
		execOpts, err := ExportECSPodExecutionOptions(evergreen.ECSConfig{}, getContainerOpts())
		assert.Error(t, err)
		assert.Zero(t, execOpts)
	})
	t.Run("FailsWithNoMatchingCluster", func(t *testing.T) {
		ecsConf := getECSConfig()
		ecsConf.Clusters[0].OS = evergreen.ECSOSWindows
		execOpts, err := ExportECSPodExecutionOptions(ecsConf, getContainerOpts())
		assert.Error(t, err)
		assert.Zero(t, execOpts)
	})
	t.Run("FailsWithNoMatchingCapacityProviderForLinux", func(t *testing.T) {
		ecsConf := getECSConfig()
		ecsConf.CapacityProviders[0].Arch = evergreen.ECSArchARM64
		execOpts, err := ExportECSPodExecutionOptions(ecsConf, getContainerOpts())
		assert.Error(t, err)
		assert.Zero(t, execOpts)
	})
	t.Run("FailsWithNoMatchingCapacityProviderForWindows", func(t *testing.T) {
		ecsConf := getECSConfig()
		containerOpts := getContainerOpts()
		containerOpts.OS = pod.OSWindows
		containerOpts.Arch = pod.ArchAMD64
		containerOpts.WindowsVersion = pod.WindowsVersionServer2016
		execOpts, err := ExportECSPodExecutionOptions(ecsConf, containerOpts)
		assert.Error(t, err)
		assert.Zero(t, execOpts)
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

func TestGetFilteredResourceIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer cocoaMock.ResetGlobalSecretCache()

	tagClient := &cocoaMock.TagClient{}
	defer func() {
		tagClient.Close(ctx)
	}()

	sc := &NoopSecretCache{Tag: "cache-tag"}

	smClient := &cocoaMock.SecretsManagerClient{}
	defer func() {
		smClient.Close(ctx)
	}()
	v, err := secret.NewBasicSecretsManager(*secret.NewBasicSecretsManagerOptions().
		SetClient(smClient).
		SetCache(sc))
	require.NoError(t, err)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, secretIDs []string){
		"ReturnsAllResultsForNoFiltersWhenNumberOfMatchesEqualsLimit": func(ctx context.Context, t *testing.T, secretIDs []string) {
			ids, err := GetFilteredResourceIDs(ctx, tagClient, nil, nil, len(secretIDs))
			require.NoError(t, err)
			assert.ElementsMatch(t, secretIDs, ids)
		},
		"ReturnsAllResultsForNegativeLimit": func(ctx context.Context, t *testing.T, secretIDs []string) {
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, nil, -1)
			require.NoError(t, err)
			assert.ElementsMatch(t, secretIDs, ids)
		},
		"ReturnsNoResultsForZeroLimit": func(ctx context.Context, t *testing.T, secretIDs []string) {
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, nil, 0)
			require.NoError(t, err)
			assert.Empty(t, ids)
		},
		"ReturnsLimitedResultsWhenNumberOfMatchesIsGreaterThanLimit": func(ctx context.Context, t *testing.T, secretIDs []string) {
			n := len(secretIDs) - 2
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, nil, n)
			require.NoError(t, err)
			assert.Subset(t, secretIDs, ids)
			assert.Len(t, ids, n)
		},
		"ReturnsAllResultsWhenNumberOfMatchesIsLessThanLimit": func(ctx context.Context, t *testing.T, secretIDs []string) {
			n := len(secretIDs) + 1
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, nil, n)
			require.NoError(t, err)
			assert.ElementsMatch(t, secretIDs, ids)
		},
		"ReturnsNoResultsForNoMatchingResourceFilter": func(ctx context.Context, t *testing.T, secretIDs []string) {
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{PodDefinitionResourceFilter}, nil, len(secretIDs))
			require.NoError(t, err)
			assert.Empty(t, ids)
		},
		"ReturnsOnlyResultsMatchingTagFilter": func(ctx context.Context, t *testing.T, secretIDs []string) {
			// Create untagged secret, which shouldn't appear in the results.
			_, err := smClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(t.Name() + utility.RandomString()),
				SecretString: aws.String(utility.RandomString()),
			})
			require.NoError(t, err)

			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, map[string][]string{
				sc.GetTag(): {strconv.FormatBool(true)},
			}, len(secretIDs)+1)
			require.NoError(t, err)
			assert.ElementsMatch(t, secretIDs, ids)
		},
		"ReturnsNoResultsForNoMatchingTagFilter": func(ctx context.Context, t *testing.T, secretIDs []string) {
			ids, err := GetFilteredResourceIDs(ctx, tagClient, []string{SecretsManagerResourceFilter}, map[string][]string{
				"nonexistent_key": {"nonexistent_value"},
			}, len(secretIDs))
			require.NoError(t, err)
			assert.Empty(t, ids)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			cocoaMock.ResetGlobalSecretCache()

			var secretIDs []string
			for i := 0; i < 5; i++ {
				id, err := v.CreateSecret(ctx, *cocoa.NewNamedSecret().
					SetName(fmt.Sprintf("%s%d", t.Name(), i)).
					SetValue(fmt.Sprintf("some_value%d", i)))
				require.NoError(t, err)
				secretIDs = append(secretIDs, id)
			}

			tCase(ctx, t, secretIDs)
		})
	}
}
