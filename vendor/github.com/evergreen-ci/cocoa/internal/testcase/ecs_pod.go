package testcase

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ECSPodTestCase represents a test case for a cocoa.ECSPod.
type ECSPodTestCase func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator)

// ECSPodTests returns common test cases that a cocoa.ECSPod should support.
func ECSPodTests() map[string]ECSPodTestCase {

	envVar := cocoa.NewEnvironmentVariable().SetName("name").SetValue("value")

	containerDef := cocoa.NewECSContainerDefinition().
		SetImage("image").
		SetEnvironmentVariables([]cocoa.EnvironmentVariable{*envVar}).
		SetMemoryMB(128).
		SetCPU(128).
		SetName("container")

	execOpts := cocoa.NewECSPodExecutionOptions().
		SetCluster(testutil.ECSClusterName()).
		SetExecutionRole(testutil.ExecutionRole())

	opts := cocoa.NewECSPodCreationOptions().
		SetName(testutil.NewTaskDefinitionFamily("name")).
		AddContainerDefinitions(*containerDef).
		SetMemoryMB(128).
		SetCPU(128).
		SetTaskRole(testutil.TaskRole()).
		SetExecutionOptions(*execOpts)

	optsSecret := cocoa.NewECSPodCreationOptions().
		SetName(testutil.NewTaskDefinitionFamily("name")).
		SetMemoryMB(128).
		SetCPU(128).
		SetTaskRole(testutil.TaskRole()).
		SetExecutionOptions(*execOpts)

	return map[string]ECSPodTestCase{
		"StopSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator) {
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)
			require.NotZero(t, p)

			require.NoError(t, p.Stop(ctx))

			info, err := p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Stopped, info.Status)
		},
		"StopSucceedsWithSecrets": func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator) {
			secret := cocoa.NewEnvironmentVariable().SetName("secret1").
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName("name1")).SetValue("value1"))
			secretOwned := cocoa.NewEnvironmentVariable().SetName("secret2").
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName("name2")).SetValue("value2"))
			secretOwned.SecretOpts.SetOwned(true)

			optsSecret := optsSecret.SetContainerDefinitions(
				[]cocoa.ECSContainerDefinition{*containerDef.SetEnvironmentVariables(
					[]cocoa.EnvironmentVariable{*secret, *secretOwned})})

			p, err := pc.CreatePod(ctx, optsSecret)
			require.NoError(t, err)
			require.NotZero(t, p)

			info, err := p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Running, info.Status)
			assert.Len(t, info.Resources.Secrets, 2)

			require.NoError(t, p.Stop(ctx))

			info, err = p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Stopped, info.Status)
			require.Len(t, info.Resources.Secrets, 2)

			arn := info.Resources.Secrets[0].Name
			id, err := v.GetValue(ctx, *arn)
			require.NoError(t, err)
			require.NotNil(t, id)
		},
		"StopFailsOnIncorrectPodStatus": func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator) {
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)
			require.NotZero(t, p)

			require.NoError(t, p.Stop(ctx))

			info, err := p.Info(ctx)
			require.NoError(t, err)
			require.NotZero(t, info)
			assert.Equal(t, cocoa.Stopped, info.Status)

			require.Error(t, p.Stop(ctx))
		},
		"DeleteSucceeds": func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator) {
			p, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)
			require.NotZero(t, p)

			info, err := p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Running, info.Status)

			require.NoError(t, p.Delete(ctx))

			info, err = p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Deleted, info.Status)
		},
		"DeleteSucceedsWithSecrets": func(ctx context.Context, t *testing.T, v cocoa.Vault, pc cocoa.ECSPodCreator) {
			secret := cocoa.NewEnvironmentVariable().SetName("secret1").
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName("name1")).SetValue("value1"))
			secretOwned := cocoa.NewEnvironmentVariable().SetName("secret2").
				SetSecretOptions(*cocoa.NewSecretOptions().SetName(testutil.NewSecretName("name2")).SetValue("value2"))
			secretOwned.SecretOpts.SetOwned(true)

			optsSecret := optsSecret.SetContainerDefinitions(
				[]cocoa.ECSContainerDefinition{*containerDef.SetEnvironmentVariables(
					[]cocoa.EnvironmentVariable{*secret, *secretOwned})})

			p, err := pc.CreatePod(ctx, optsSecret)
			require.NoError(t, err)
			require.NotZero(t, p)

			info, err := p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Running, info.Status)
			require.Len(t, info.Resources.Secrets, 2)

			require.NoError(t, p.Delete(ctx))

			info, err = p.Info(ctx)
			require.NoError(t, err)
			require.NotNil(t, info)
			assert.Equal(t, cocoa.Deleted, info.Status)
			require.Len(t, info.Resources.Secrets, 2)

			arn0 := info.Resources.Secrets[0].Name
			val, err := v.GetValue(ctx, *arn0)
			require.NoError(t, err)
			require.NotZero(t, val)
			assert.Equal(t, *info.Resources.Secrets[0].NamedSecret.Value, *secret.SecretOpts.NamedSecret.Value)

			arn1 := info.Resources.Secrets[1].Name
			_, err = v.GetValue(ctx, *arn1)
			require.Error(t, err)
		},
	}
}
