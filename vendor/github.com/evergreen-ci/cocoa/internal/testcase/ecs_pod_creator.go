package testcase

import (
	"context"
	"testing"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ECSPodCreatorTestCase represents a test case for a cocoa.ECSPodCreator.
type ECSPodCreatorTestCase func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator)

// ECSPodCreatorTests returns common test cases that a cocoa.ECSPodCreator should support.
func ECSPodCreatorTests() map[string]ECSPodCreatorTestCase {
	return map[string]ECSPodCreatorTestCase{
		"CreatePodSucceedsWithNonSecretSettings": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			envVar := cocoa.NewEnvironmentVariable().SetName("name").SetValue("value")
			containerDef := cocoa.NewECSContainerDefinition().
				SetImage("image").
				SetWorkingDir("working_dir").
				AddEnvironmentVariables(*envVar).
				SetMemoryMB(128).
				SetCPU(128).
				AddPortMappings(*cocoa.NewPortMapping().SetContainerPort(1337)).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				SetName(testutil.NewTaskDefinitionFamily(t)).
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetNetworkMode(cocoa.NetworkModeBridge).
				SetExecutionOptions(*execOpts)
			assert.NoError(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.NoError(t, err)
			require.NotNil(t, p)

			defer func() {
				require.NoError(t, p.Delete(ctx))
			}()

			ps := p.StatusInfo()
			assert.Equal(t, cocoa.StatusStarting, ps.Status)
		},
		"CreatePodFailsWithInvalidCreationOpts": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			opts := cocoa.NewECSPodCreationOptions()

			p, err := c.CreatePod(ctx, *opts)
			require.Error(t, err)
			require.Zero(t, p)
		},
		"CreatePodFailsWithSecretsButNoVault": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			envVar := cocoa.NewEnvironmentVariable().
				SetName("envVar").
				SetSecretOptions(*cocoa.NewSecretOptions().
					SetName(testutil.NewSecretName(t)).
					SetNewValue("value"))
			containerDef := cocoa.NewECSContainerDefinition().
				SetImage("image").
				AddEnvironmentVariables(*envVar).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				SetName(testutil.NewTaskDefinitionFamily(t)).
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole(testutil.ECSTaskRole()).
				SetExecutionRole(testutil.ECSExecutionRole()).
				SetExecutionOptions(*execOpts)
			assert.NoError(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.Error(t, err)
			require.Zero(t, p)
		},
		"CreatePodFailsWithRepoCredsButNoVault": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			storedCreds := cocoa.NewStoredRepositoryCredentials().
				SetUsername("username").
				SetPassword("password")
			creds := cocoa.NewRepositoryCredentials().
				SetName(testutil.NewSecretName(t)).
				SetNewCredentials(*storedCreds)
			containerDef := cocoa.NewECSContainerDefinition().
				SetImage("image").
				SetRepositoryCredentials(*creds).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				SetName(testutil.NewTaskDefinitionFamily(t)).
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole(testutil.ECSTaskRole()).
				SetExecutionRole(testutil.ECSExecutionRole()).
				SetExecutionOptions(*execOpts)
			assert.NoError(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.Error(t, err)
			require.Zero(t, p)
		},
	}
}

// ECSPodCreatorWithVaultTests returns common test cases that a cocoa.ECSPodCreator should support with a Vault.
func ECSPodCreatorWithVaultTests() map[string]ECSPodCreatorTestCase {
	return map[string]ECSPodCreatorTestCase{
		"CreatePodFailsWithSecretsButNoExecutionRole": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			envVar := cocoa.NewEnvironmentVariable().SetName("envVar").
				SetSecretOptions(*cocoa.NewSecretOptions().
					SetName(testutil.NewSecretName(t)).
					SetNewValue("value"))
			containerDef := cocoa.NewECSContainerDefinition().SetImage("image").
				AddEnvironmentVariables(*envVar).
				SetMemoryMB(128).
				SetCPU(128).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole(testutil.ECSTaskRole()).
				SetExecutionOptions(*execOpts).
				SetName(testutil.NewTaskDefinitionFamily(t))
			assert.Error(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.Error(t, err)
			require.Zero(t, p)
		},
		"CreatePodSucceedsWithNewlyCreatedSecrets": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			envVar := cocoa.NewEnvironmentVariable().
				SetName("envVar").
				SetSecretOptions(*cocoa.NewSecretOptions().
					SetName(testutil.NewSecretName(t)).
					SetNewValue("value").
					SetOwned(true))

			containerDef := cocoa.NewECSContainerDefinition().
				SetImage("image").
				AddEnvironmentVariables(*envVar).
				SetMemoryMB(128).
				SetCPU(128).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				SetName(testutil.NewTaskDefinitionFamily(t)).
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole(testutil.ECSTaskRole()).
				SetExecutionRole(testutil.ECSExecutionRole()).
				SetExecutionOptions(*execOpts)
			assert.NoError(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.NoError(t, err)
			require.NotNil(t, p)

			defer func() {
				require.NoError(t, p.Delete(ctx))
			}()

			checkPodStatus(t, p, cocoa.StatusStarting)
		},
		"CreatePodSucceedsWithNewlyCreatedRepoCreds": func(ctx context.Context, t *testing.T, c cocoa.ECSPodCreator) {
			storedCreds := cocoa.NewStoredRepositoryCredentials().
				SetUsername("username").
				SetPassword("password")
			creds := cocoa.NewRepositoryCredentials().
				SetName(testutil.NewSecretName(t)).
				SetNewCredentials(*storedCreds).
				SetOwned(true)

			containerDef := cocoa.NewECSContainerDefinition().
				SetImage("image").
				SetRepositoryCredentials(*creds).
				SetMemoryMB(128).
				SetCPU(128).
				SetName("container")

			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName())

			opts := cocoa.NewECSPodCreationOptions().
				SetName(testutil.NewTaskDefinitionFamily(t)).
				AddContainerDefinitions(*containerDef).
				SetMemoryMB(128).
				SetCPU(128).
				SetTaskRole(testutil.ECSTaskRole()).
				SetExecutionRole(testutil.ECSExecutionRole()).
				SetExecutionOptions(*execOpts)
			assert.NoError(t, opts.Validate())

			p, err := c.CreatePod(ctx, *opts)
			require.NoError(t, err)
			require.NotNil(t, p)

			defer func() {
				require.NoError(t, p.Delete(ctx))
			}()

			checkPodStatus(t, p, cocoa.StatusStarting)
		},
	}
}
