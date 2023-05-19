package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPICreatePod(t *testing.T) {
	t.Run("ToService", func(t *testing.T) {
		apiPod := APICreatePod{
			Name:                utility.ToStringPtr("id"),
			Memory:              utility.ToIntPtr(128),
			CPU:                 utility.ToIntPtr(128),
			Image:               utility.ToStringPtr("image"),
			RepoCredsExternalID: utility.ToStringPtr("repo_creds_external_id"),
			OS:                  APIPodOS(pod.OSWindows),
			Arch:                APIPodArch(pod.ArchAMD64),
			WindowsVersion:      APIPodWindowsVersion(pod.WindowsVersionServer2019),
			PodSecretExternalID: utility.ToStringPtr("pod_secret_external_id"),
			PodSecretValue:      utility.ToStringPtr("pod_secret_value"),
			WorkingDir:          utility.ToStringPtr("working_dir"),
		}

		evergreen.GetEnvironment().Settings().Providers.AWS.Pod.ECS = evergreen.ECSConfig{
			AllowedImages: []string{
				"image",
			},
		}
		p, err := apiPod.ToService()
		require.NoError(t, err)

		assert.Equal(t, utility.FromStringPtr(apiPod.Image), p.TaskContainerCreationOpts.Image)
		assert.Equal(t, utility.FromStringPtr(apiPod.RepoCredsExternalID), p.TaskContainerCreationOpts.RepoCredsExternalID)
		assert.Equal(t, utility.FromIntPtr(apiPod.Memory), p.TaskContainerCreationOpts.MemoryMB)
		assert.Equal(t, utility.FromIntPtr(apiPod.CPU), p.TaskContainerCreationOpts.CPU)
		assert.EqualValues(t, apiPod.OS, p.TaskContainerCreationOpts.OS)
		assert.EqualValues(t, apiPod.Arch, p.TaskContainerCreationOpts.Arch)
		require.NotZero(t, p.TaskContainerCreationOpts.WindowsVersion)
		assert.EqualValues(t, apiPod.WindowsVersion, p.TaskContainerCreationOpts.WindowsVersion)
		assert.EqualValues(t, utility.FromStringPtr(apiPod.WorkingDir), p.TaskContainerCreationOpts.WorkingDir)
		assert.Equal(t, pod.StatusInitializing, p.Status)
		assert.NotZero(t, p.TimeInfo.Initializing)
		assert.Len(t, p.TaskContainerCreationOpts.EnvVars, 1, "pod ID should be set")
		assert.Len(t, p.TaskContainerCreationOpts.EnvSecrets, 1, "pod secret should be set")
		podID, ok := p.TaskContainerCreationOpts.EnvVars["POD_ID"]
		require.True(t, ok)
		assert.NotZero(t, p.ID)
		assert.Equal(t, p.ID, podID)
		assert.Equal(t, utility.FromStringPtr(apiPod.RepoCredsExternalID), p.TaskContainerCreationOpts.RepoCredsExternalID)
		podSecret, err := p.GetSecret()
		require.NoError(t, err)
		assert.Equal(t, utility.FromStringPtr(apiPod.PodSecretExternalID), podSecret.ExternalID)
	})
}

func TestAPIPod(t *testing.T) {
	validDBPod := func() pod.Pod {
		return pod.Pod{
			ID:     "id",
			Type:   pod.TypeAgent,
			Status: pod.StatusRunning,
			TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
				Image:          "image",
				MemoryMB:       128,
				CPU:            128,
				OS:             pod.OSWindows,
				Arch:           pod.ArchAMD64,
				WindowsVersion: pod.WindowsVersionServer2019,
				EnvVars: map[string]string{
					"ENV_VAR0": "val0",
					"ENV_VAR1": "val1",
				},
				EnvSecrets: map[string]pod.Secret{
					"SECRET_ENV_VAR0": {
						ExternalID: "external_id0",
						Value:      "secret_val0",
					},
					"SECRET_ENV_VAR1": {
						ExternalID: "external_id1",
						Value:      "secret_val1",
					},
				},
				WorkingDir: "working_dir",
			},
			Family: "family",
			TimeInfo: pod.TimeInfo{
				Initializing:     time.Now(),
				Starting:         time.Now(),
				LastCommunicated: time.Now(),
			},
			Resources: pod.ResourceInfo{
				ExternalID:   "task_id",
				DefinitionID: "definition_id",
				Cluster:      "cluster",
				Containers: []pod.ContainerResourceInfo{
					{
						ExternalID: "container_id",
						Name:       "container_name",
						SecretIDs:  []string{"secret_id0", "secret_id1"},
					},
				},
			},
			TaskRuntimeInfo: pod.TaskRuntimeInfo{
				RunningTaskID:        "task_id",
				RunningTaskExecution: 3,
			},
			AgentVersion: "2020-01-01",
		}
	}

	validAPIPod := func() APIPod {
		return APIPod{
			ID:     utility.ToStringPtr("id"),
			Type:   PodTypeAgent,
			Status: PodStatusRunning,
			TaskContainerCreationOpts: APIPodTaskContainerCreationOptions{
				Image:               utility.ToStringPtr("image"),
				RepoCredsExternalID: utility.ToStringPtr("repo_creds_external_id"),
				MemoryMB:            utility.ToIntPtr(128),
				CPU:                 utility.ToIntPtr(128),
				OS:                  APIPodOS(pod.OSWindows),
				Arch:                APIPodArch(pod.ArchAMD64),
				WindowsVersion:      APIPodWindowsVersion(pod.WindowsVersionServer2019),
				EnvVars: map[string]string{
					"ENV_VAR0": "val0",
					"ENV_VAR1": "val1",
				},
				EnvSecrets: map[string]APIPodSecret{
					"SECRET_ENV_VAR0": {
						ExternalID: utility.ToStringPtr("external_id0"),
						Value:      utility.ToStringPtr("secret_val0"),
					},
					"SECRET_ENV_VAR1": {
						ExternalID: utility.ToStringPtr("external_id1"),
						Value:      utility.ToStringPtr("secret_val0"),
					},
				},
				WorkingDir: utility.ToStringPtr("working_dir"),
			},
			Family: utility.ToStringPtr("family"),
			TimeInfo: APIPodTimeInfo{
				Initializing:     utility.ToTimePtr(time.Now()),
				Starting:         utility.ToTimePtr(time.Now()),
				LastCommunicated: utility.ToTimePtr(time.Now()),
			},
			Resources: APIPodResourceInfo{
				ExternalID:   utility.ToStringPtr("task_id"),
				DefinitionID: utility.ToStringPtr("definition_id"),
				Cluster:      utility.ToStringPtr("cluster"),
				Containers: []APIContainerResourceInfo{
					{
						ExternalID: utility.ToStringPtr("container_id"),
						Name:       utility.ToStringPtr("container_name"),
						SecretIDs:  []string{"secret_id0", "secret_id1"},
					},
				},
			},
			TaskRuntimeInfo: APITaskRuntimeInfo{
				RunningTaskID:        utility.ToStringPtr("task_id"),
				RunningTaskExecution: utility.ToIntPtr(3),
			},
			AgentVersion: utility.ToStringPtr("2020-01-01"),
		}
	}
	t.Run("ToService", func(t *testing.T) {
		t.Run("Succeeds", func(t *testing.T) {
			apiPod := validAPIPod()
			dbPod, err := apiPod.ToService()
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(apiPod.ID), dbPod.ID)
			assert.Equal(t, pod.TypeAgent, dbPod.Type)
			assert.Equal(t, pod.StatusRunning, dbPod.Status)
			assert.Equal(t, utility.FromStringPtr(apiPod.Family), dbPod.Family)
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.Image), dbPod.TaskContainerCreationOpts.Image)
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoCredsExternalID), dbPod.TaskContainerCreationOpts.RepoCredsExternalID)
			assert.Equal(t, utility.FromIntPtr(apiPod.TaskContainerCreationOpts.MemoryMB), dbPod.TaskContainerCreationOpts.MemoryMB)
			assert.Equal(t, utility.FromIntPtr(apiPod.TaskContainerCreationOpts.CPU), dbPod.TaskContainerCreationOpts.CPU)
			assert.EqualValues(t, apiPod.TaskContainerCreationOpts.OS, dbPod.TaskContainerCreationOpts.OS)
			assert.EqualValues(t, apiPod.TaskContainerCreationOpts.Arch, dbPod.TaskContainerCreationOpts.Arch)
			assert.EqualValues(t, apiPod.TaskContainerCreationOpts.WindowsVersion, dbPod.TaskContainerCreationOpts.WindowsVersion)
			require.NotZero(t, dbPod.TaskContainerCreationOpts.EnvVars)
			for k, v := range apiPod.TaskContainerCreationOpts.EnvVars {
				assert.Equal(t, v, dbPod.TaskContainerCreationOpts.EnvVars[k])
			}
			require.NotZero(t, dbPod.TaskContainerCreationOpts.EnvSecrets)
			for k, v := range apiPod.TaskContainerCreationOpts.EnvSecrets {
				dbPodSecret, ok := dbPod.TaskContainerCreationOpts.EnvSecrets[k]
				require.True(t, ok)
				assert.Equal(t, v.ToService(), dbPodSecret)
			}
			assert.Equal(t, utility.FromTimePtr(apiPod.TimeInfo.Initializing), dbPod.TimeInfo.Initializing)
			assert.Equal(t, utility.FromTimePtr(apiPod.TimeInfo.Starting), dbPod.TimeInfo.Starting)
			assert.Equal(t, utility.FromTimePtr(apiPod.TimeInfo.LastCommunicated), dbPod.TimeInfo.LastCommunicated)
			assert.Equal(t, utility.FromStringPtr(apiPod.Resources.ExternalID), dbPod.Resources.ExternalID)
			assert.Equal(t, utility.FromStringPtr(apiPod.Resources.DefinitionID), dbPod.Resources.DefinitionID)
			assert.Equal(t, utility.FromStringPtr(apiPod.Resources.Cluster), dbPod.Resources.Cluster)
			require.Len(t, dbPod.Resources.Containers, len(apiPod.Resources.Containers))
			for i := range apiPod.Resources.Containers {
				assert.Equal(t, utility.FromStringPtr(apiPod.Resources.Containers[i].ExternalID), dbPod.Resources.Containers[i].ExternalID)
				assert.Equal(t, utility.FromStringPtr(apiPod.Resources.Containers[i].Name), dbPod.Resources.Containers[i].Name)
				left, right := utility.StringSliceSymmetricDifference(apiPod.Resources.Containers[i].SecretIDs, dbPod.Resources.Containers[i].SecretIDs)
				assert.Empty(t, left, "actual is missing container secret IDs: %s", left)
				assert.Empty(t, right, "actual has extra unexpected container secret IDs: %s", right)
			}
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskRuntimeInfo.RunningTaskID), dbPod.TaskRuntimeInfo.RunningTaskID)
			assert.Equal(t, utility.FromIntPtr(apiPod.TaskRuntimeInfo.RunningTaskExecution), dbPod.TaskRuntimeInfo.RunningTaskExecution)
			assert.Equal(t, utility.FromStringPtr(apiPod.AgentVersion), dbPod.AgentVersion)
		})
		t.Run("FailsWithInvalidPodType", func(t *testing.T) {
			apiPod := validAPIPod()
			apiPod.Type = "foo"
			_, err := apiPod.ToService()
			assert.Error(t, err)
		})
		t.Run("FailsWithInvalidStatus", func(t *testing.T) {
			apiPod := validAPIPod()
			apiPod.Status = "foo"
			_, err := apiPod.ToService()
			assert.Error(t, err)
		})
		t.Run("FailsWithInvalidOS", func(t *testing.T) {
			apiPod := validAPIPod()
			apiPod.TaskContainerCreationOpts.OS = ""
			_, err := apiPod.ToService()
			assert.Error(t, err)
		})
		t.Run("FailsWithInvalidArch", func(t *testing.T) {
			apiPod := validAPIPod()
			apiPod.TaskContainerCreationOpts.Arch = ""
			_, err := apiPod.ToService()
			assert.Error(t, err)
		})
	})
	t.Run("BuildFromService", func(t *testing.T) {
		t.Run("Succeeds", func(t *testing.T) {
			dbPod := validDBPod()
			var apiPod APIPod
			require.NoError(t, apiPod.BuildFromService(&dbPod))
			assert.Equal(t, dbPod.ID, utility.FromStringPtr(apiPod.ID))
			assert.Equal(t, dbPod.Family, utility.FromStringPtr(apiPod.Family))
			assert.Equal(t, dbPod.AgentVersion, utility.FromStringPtr(apiPod.AgentVersion))
			assert.Equal(t, PodTypeAgent, apiPod.Type)
			assert.Equal(t, PodStatusRunning, apiPod.Status)
			assert.Equal(t, dbPod.TaskContainerCreationOpts.Image, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.Image))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.RepoCredsExternalID, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoCredsExternalID))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.MemoryMB, utility.FromIntPtr(apiPod.TaskContainerCreationOpts.MemoryMB))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.CPU, utility.FromIntPtr(apiPod.TaskContainerCreationOpts.CPU))
			assert.EqualValues(t, dbPod.TaskContainerCreationOpts.OS, apiPod.TaskContainerCreationOpts.OS)
			assert.EqualValues(t, dbPod.TaskContainerCreationOpts.Arch, apiPod.TaskContainerCreationOpts.Arch)
			assert.EqualValues(t, dbPod.TaskContainerCreationOpts.WindowsVersion, apiPod.TaskContainerCreationOpts.WindowsVersion)
			assert.Equal(t, dbPod.TaskContainerCreationOpts.WorkingDir, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.WorkingDir))
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars)
			for k, v := range dbPod.TaskContainerCreationOpts.EnvVars {
				assert.Equal(t, v, apiPod.TaskContainerCreationOpts.EnvVars[k])
			}
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvSecrets)
			for k, v := range dbPod.TaskContainerCreationOpts.EnvSecrets {
				apiPodSecret, ok := apiPod.TaskContainerCreationOpts.EnvSecrets[k]
				require.True(t, ok)
				assert.Equal(t, v, apiPodSecret.ToService())
			}
			assert.Equal(t, dbPod.TimeInfo.Initializing, utility.FromTimePtr(apiPod.TimeInfo.Initializing))
			assert.Equal(t, dbPod.TimeInfo.Starting, utility.FromTimePtr(apiPod.TimeInfo.Starting))
			assert.Equal(t, dbPod.TimeInfo.LastCommunicated, utility.FromTimePtr(apiPod.TimeInfo.LastCommunicated))
			assert.Equal(t, dbPod.Resources.ExternalID, utility.FromStringPtr(apiPod.Resources.ExternalID))
			assert.Equal(t, dbPod.Resources.DefinitionID, utility.FromStringPtr(apiPod.Resources.DefinitionID))
			assert.Equal(t, dbPod.Resources.Cluster, utility.FromStringPtr(apiPod.Resources.Cluster))
			require.Len(t, apiPod.Resources.Containers, len(dbPod.Resources.Containers))
			for i := range dbPod.Resources.Containers {
				assert.Equal(t, dbPod.Resources.Containers[i].ExternalID, utility.FromStringPtr(apiPod.Resources.Containers[i].ExternalID))
				assert.Equal(t, dbPod.Resources.Containers[i].Name, utility.FromStringPtr(apiPod.Resources.Containers[i].Name))
				left, right := utility.StringSliceSymmetricDifference(dbPod.Resources.Containers[i].SecretIDs, apiPod.Resources.Containers[i].SecretIDs)
				assert.Empty(t, left, "actual is missing container secret IDs: %s", left)
				assert.Empty(t, right, "actual has extra unexpected container secret IDs: %s", right)
			}
			assert.Equal(t, dbPod.TaskRuntimeInfo.RunningTaskID, utility.FromStringPtr(apiPod.TaskRuntimeInfo.RunningTaskID))
			assert.Equal(t, dbPod.TaskRuntimeInfo.RunningTaskExecution, utility.FromIntPtr(apiPod.TaskRuntimeInfo.RunningTaskExecution))
		})
		t.Run("FailsWithInvalidPodType", func(t *testing.T) {
			dbPod := validDBPod()
			dbPod.Type = "invalid"
			var apiPod APIPod
			assert.Error(t, apiPod.BuildFromService(&dbPod))
		})
		t.Run("FailsWithInvalidStatus", func(t *testing.T) {
			dbPod := validDBPod()
			dbPod.Status = "invalid"
			var apiPod APIPod
			assert.Error(t, apiPod.BuildFromService(&dbPod))
		})
	})
}
