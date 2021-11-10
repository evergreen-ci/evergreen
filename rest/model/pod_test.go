package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPICreatePod(t *testing.T) {
	t.Run("ToService", func(t *testing.T) {
		apiPod := APICreatePod{
			Name:         utility.ToStringPtr("id"),
			Memory:       utility.ToIntPtr(128),
			CPU:          utility.ToIntPtr(128),
			Image:        utility.ToStringPtr("image"),
			RepoUsername: utility.ToStringPtr("username"),
			RepoPassword: utility.ToStringPtr("password"),
			EnvVars: []*APIPodEnvVar{
				{
					Name:   utility.ToStringPtr("name"),
					Value:  utility.ToStringPtr("value"),
					Secret: utility.ToBoolPtr(false),
				},
				{
					Name:   utility.ToStringPtr("name1"),
					Value:  utility.ToStringPtr("value1"),
					Secret: utility.ToBoolPtr(false),
				},
				{
					Name:   utility.ToStringPtr("secret_name"),
					Value:  utility.ToStringPtr("secret_value"),
					Secret: utility.ToBoolPtr(true),
				},
			},
			OS:             APIPodOS(pod.OSWindows),
			Arch:           APIPodArch(pod.ArchAMD64),
			WindowsVersion: APIPodWindowsVersion(pod.WindowsVersionServer2019),
			Secret:         utility.ToStringPtr("secret"),
			WorkingDir:     utility.ToStringPtr("working_dir"),
		}

		res, err := apiPod.ToService()
		require.NoError(t, err)

		p, ok := res.(pod.Pod)
		require.True(t, ok)

		assert.Zero(t, p.Secret.Name)
		assert.Zero(t, p.Secret.ExternalID)
		assert.Equal(t, utility.FromStringPtr(apiPod.Secret), p.Secret.Value)
		assert.False(t, utility.FromBoolPtr(p.Secret.Exists))
		assert.True(t, utility.FromBoolPtr(p.Secret.Owned))
		assert.Equal(t, utility.FromStringPtr(apiPod.Image), p.TaskContainerCreationOpts.Image)
		assert.Equal(t, utility.FromStringPtr(apiPod.RepoUsername), p.TaskContainerCreationOpts.RepoUsername)
		assert.Equal(t, utility.FromStringPtr(apiPod.RepoPassword), p.TaskContainerCreationOpts.RepoPassword)
		assert.Equal(t, utility.FromIntPtr(apiPod.Memory), p.TaskContainerCreationOpts.MemoryMB)
		assert.Equal(t, utility.FromIntPtr(apiPod.CPU), p.TaskContainerCreationOpts.CPU)
		assert.EqualValues(t, apiPod.OS, p.TaskContainerCreationOpts.OS)
		assert.EqualValues(t, apiPod.Arch, p.TaskContainerCreationOpts.Arch)
		require.NotZero(t, p.TaskContainerCreationOpts.WindowsVersion)
		assert.EqualValues(t, apiPod.WindowsVersion, p.TaskContainerCreationOpts.WindowsVersion)
		assert.EqualValues(t, utility.FromStringPtr(apiPod.WorkingDir), p.TaskContainerCreationOpts.WorkingDir)
		assert.Equal(t, pod.StatusInitializing, p.Status)
		assert.NotZero(t, p.TimeInfo.Initializing)
		assert.Len(t, p.TaskContainerCreationOpts.EnvVars, 2)
		assert.Len(t, p.TaskContainerCreationOpts.EnvSecrets, 1)
		assert.Equal(t, "value1", p.TaskContainerCreationOpts.EnvVars["name1"])
		dbPodSecret, ok := p.TaskContainerCreationOpts.EnvSecrets["secret_name"]
		require.True(t, ok)
		assert.Equal(t, "secret_name", dbPodSecret.Name)
		assert.Zero(t, dbPodSecret.ExternalID)
		assert.Equal(t, "secret_value", dbPodSecret.Value)
		assert.False(t, utility.FromBoolPtr(dbPodSecret.Exists))
		assert.True(t, utility.FromBoolPtr(dbPodSecret.Owned))
	})
}

func TestAPIPod(t *testing.T) {
	validDBPod := func() pod.Pod {
		return pod.Pod{
			ID:     "id",
			Status: pod.StatusRunning,
			Secret: pod.Secret{
				Name:       "secret_name",
				ExternalID: "external_id",
				Value:      "secret_value",
				Exists:     utility.TruePtr(),
				Owned:      utility.FalsePtr(),
			},
			TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
				Image:          "image",
				RepoUsername:   "username",
				RepoPassword:   "password",
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
						Name:   "secret_name0",
						Value:  "secret_val0",
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
					"SECRET_ENV_VAR1": {
						Name:   "secret_name1",
						Value:  "secret_val1",
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
				},
				WorkingDir: "working_dir",
			},
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
		}
	}

	validAPIPod := func() APIPod {
		return APIPod{
			ID:     utility.ToStringPtr("id"),
			Status: PodStatusRunning,
			Secret: APIPodSecret{
				Name:       utility.ToStringPtr("secret_name"),
				ExternalID: utility.ToStringPtr("external_id"),
				Value:      utility.ToStringPtr("secret_value"),
				Exists:     utility.TruePtr(),
				Owned:      utility.FalsePtr(),
			},
			TaskContainerCreationOpts: APIPodTaskContainerCreationOptions{
				Image:          utility.ToStringPtr("image"),
				RepoUsername:   utility.ToStringPtr("username"),
				RepoPassword:   utility.ToStringPtr("password"),
				MemoryMB:       utility.ToIntPtr(128),
				CPU:            utility.ToIntPtr(128),
				OS:             APIPodOS(pod.OSWindows),
				Arch:           APIPodArch(pod.ArchAMD64),
				WindowsVersion: APIPodWindowsVersion(pod.WindowsVersionServer2019),
				EnvVars: map[string]string{
					"ENV_VAR0": "val0",
					"ENV_VAR1": "val1",
				},
				EnvSecrets: map[string]APIPodSecret{
					"SECRET_ENV_VAR0": {
						Name:   utility.ToStringPtr("secret_name0"),
						Value:  utility.ToStringPtr("secret_val0"),
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
					"SECRET_ENV_VAR1": {
						Name:   utility.ToStringPtr("secret_name0"),
						Value:  utility.ToStringPtr("secret_val0"),
						Exists: utility.FalsePtr(),
						Owned:  utility.TruePtr(),
					},
				},
				WorkingDir: utility.ToStringPtr("working_dir"),
			},
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
		}
	}
	t.Run("ToService", func(t *testing.T) {
		t.Run("Succeeds", func(t *testing.T) {
			apiPod := validAPIPod()
			dbPod, err := apiPod.ToService()
			require.NoError(t, err)
			assert.Equal(t, utility.FromStringPtr(apiPod.ID), dbPod.ID)
			assert.Equal(t, pod.StatusRunning, dbPod.Status)
			assert.Equal(t, utility.FromStringPtr(apiPod.Secret.Name), dbPod.Secret.Name)
			assert.Equal(t, utility.FromStringPtr(apiPod.Secret.ExternalID), dbPod.Secret.ExternalID)
			assert.Equal(t, utility.FromStringPtr(apiPod.Secret.Value), dbPod.Secret.Value)
			assert.Equal(t, utility.FromBoolPtr(apiPod.Secret.Exists), utility.FromBoolPtr(dbPod.Secret.Exists))
			assert.Equal(t, utility.FromBoolPtr(apiPod.Secret.Owned), utility.FromBoolPtr(dbPod.Secret.Owned))
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.Image), dbPod.TaskContainerCreationOpts.Image)
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoUsername), dbPod.TaskContainerCreationOpts.RepoUsername)
			assert.Equal(t, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoPassword), dbPod.TaskContainerCreationOpts.RepoPassword)
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
			require.NotZero(t, apiPod.Status)
			assert.Equal(t, PodStatusRunning, apiPod.Status)
			assert.Equal(t, dbPod.Secret.Name, utility.FromStringPtr(apiPod.Secret.Name))
			assert.Equal(t, dbPod.Secret.ExternalID, utility.FromStringPtr(apiPod.Secret.ExternalID))
			assert.Equal(t, dbPod.Secret.Value, utility.FromStringPtr(apiPod.Secret.Value))
			assert.Equal(t, utility.FromBoolPtr(dbPod.Secret.Exists), utility.FromBoolPtr(apiPod.Secret.Exists))
			assert.Equal(t, utility.FromBoolPtr(dbPod.Secret.Owned), utility.FromBoolPtr(apiPod.Secret.Owned))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.Image, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.Image))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.RepoUsername, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoUsername))
			assert.Equal(t, dbPod.TaskContainerCreationOpts.RepoPassword, utility.FromStringPtr(apiPod.TaskContainerCreationOpts.RepoPassword))
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
		})
		t.Run("FailsWithInvalidStatus", func(t *testing.T) {
			dbPod := validDBPod()
			dbPod.Status = "invalid"
			var apiPod APIPod
			assert.Error(t, apiPod.BuildFromService(&dbPod))
		})
	})
}
