package mock

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSPodCreator(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPodCreator)(nil), &ECSPodCreator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	GlobalECSService.Clusters[testutil.ECSClusterName()] = ECSCluster{}

	for tName, tCase := range ecsPodCreatorInputTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			sm := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, sm.Close(tctx))
			}()

			v := NewVault(secret.NewBasicSecretsManager(sm))

			pc, err := ecs.NewBasicECSPodCreator(c, v)
			require.NoError(t, err)

			mpc := NewECSPodCreator(pc)

			tCase(tctx, t, mpc, c, sm)
		})
	}

	for tName, tCase := range testcase.ECSPodCreatorTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			pc, err := ecs.NewBasicECSPodCreator(c, nil)
			require.NoError(t, err)

			podCreator := NewECSPodCreator(pc)

			tCase(tctx, t, podCreator)
		})
	}

	for tName, tCase := range testcase.ECSPodCreatorWithVaultTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			sm := &SecretsManagerClient{}
			defer func() {
				assert.NoError(t, sm.Close(tctx))
			}()

			v := NewVault(secret.NewBasicSecretsManager(sm))

			pc, err := ecs.NewBasicECSPodCreator(c, v)
			require.NoError(t, err)

			podCreator := NewECSPodCreator(pc)

			tCase(tctx, t, podCreator)
		})
	}
}

func ecsPodCreatorInputTests() map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
	return map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient){
		"RegistersTaskDefinitionAndRunsTaskWithAllFieldsSet": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
			envVar := cocoa.NewEnvironmentVariable().
				SetName("env_var_name").
				SetValue("env_var_value")
			containerDef := cocoa.NewECSContainerDefinition().
				SetName("name").
				SetImage("image").
				SetCommand([]string{"echo", "foo"}).
				SetWorkingDir("working_dir").
				SetMemoryMB(128).
				SetCPU(256).
				AddEnvironmentVariables(*envVar)
			placementOpts := cocoa.NewECSPodPlacementOptions().
				SetStrategy(cocoa.StrategyBinpack).
				SetStrategyParameter(cocoa.StrategyParamBinpackMemory)
			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName()).
				SetPlacementOptions(*placementOpts).
				SetTags(map[string]string{"execution_tag": "execution_val"}).
				SetSupportsDebugMode(true)
			opts := cocoa.NewECSPodCreationOptions().
				SetMemoryMB(512).
				SetCPU(1024).
				SetTaskRole("task_role").
				SetExecutionRole("execution_role").
				SetTags(map[string]string{"creation_tag": "creation_val"}).
				AddContainerDefinitions(*containerDef).
				SetExecutionOptions(*execOpts)

			_, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			require.NotZero(t, c.RegisterTaskDefinitionInput)

			mem, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Memory))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.MemoryMB), mem)
			cpu, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Cpu))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.CPU), cpu)
			assert.Equal(t, utility.FromStringPtr(opts.TaskRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.TaskRoleArn))
			assert.Equal(t, utility.FromStringPtr(opts.ExecutionRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ExecutionRoleArn))
			require.Len(t, c.RegisterTaskDefinitionInput.Tags, 1)
			assert.Equal(t, "creation_tag", utility.FromStringPtr(c.RegisterTaskDefinitionInput.Tags[0].Key))
			assert.Equal(t, opts.Tags["creation_tag"], utility.FromStringPtr(c.RegisterTaskDefinitionInput.Tags[0].Value))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions, 1)
			left, right := utility.StringSliceSymmetricDifference(containerDef.Command, utility.FromStringPtrSlice(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Command))
			assert.Empty(t, left)
			assert.Empty(t, right)
			assert.Equal(t, utility.FromStringPtr(containerDef.WorkingDir), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].WorkingDirectory))
			assert.EqualValues(t, utility.FromIntPtr(containerDef.MemoryMB), utility.FromInt64Ptr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Memory))
			assert.EqualValues(t, utility.FromIntPtr(containerDef.CPU), utility.FromInt64Ptr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Cpu))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment, 1)
			assert.Equal(t, utility.FromStringPtr(envVar.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment[0].Name))
			assert.Equal(t, utility.FromStringPtr(envVar.Value), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment[0].Value))

			require.NotZero(t, c.RunTaskInput)
			require.Len(t, c.RunTaskInput.PlacementStrategy, 1)
			assert.EqualValues(t, *placementOpts.Strategy, utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Type))
			assert.Equal(t, utility.FromStringPtr(placementOpts.StrategyParameter), utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Field))
			require.Len(t, c.RunTaskInput.Tags, 1)
			assert.Equal(t, "execution_tag", utility.FromStringPtr(c.RunTaskInput.Tags[0].Key))
			assert.Equal(t, execOpts.Tags["execution_tag"], utility.FromStringPtr(c.RunTaskInput.Tags[0].Value))
			assert.True(t, utility.FromBoolPtr(c.RunTaskInput.EnableExecuteCommand))
		},
		"RegistersTaskDefinitionAndRunsTaskWithCreatedSecrets": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
			secretOpts := cocoa.NewSecretOptions().
				SetName("secret_name").
				SetValue("secret_value")
			envVar := cocoa.NewEnvironmentVariable().
				SetName("env_var_name").
				SetSecretOptions(*secretOpts)
			containerDef := cocoa.NewECSContainerDefinition().
				SetName("name").
				SetImage("image").
				SetCommand([]string{"echo", "foo"}).
				AddEnvironmentVariables(*envVar)
			placementOpts := cocoa.NewECSPodPlacementOptions().
				SetStrategy(cocoa.StrategyBinpack).
				SetStrategyParameter(cocoa.StrategyParamBinpackMemory)
			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName()).
				SetPlacementOptions(*placementOpts)
			opts := cocoa.NewECSPodCreationOptions().
				SetMemoryMB(512).
				SetCPU(1024).
				SetExecutionRole("execution_role").
				AddContainerDefinitions(*containerDef).
				SetExecutionOptions(*execOpts)

			_, err := pc.CreatePod(ctx, opts)
			require.NoError(t, err)

			require.NotZero(t, c.RegisterTaskDefinitionInput)

			mem, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Memory))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.MemoryMB), mem)
			cpu, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Cpu))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.CPU), cpu)
			assert.Equal(t, utility.FromStringPtr(opts.TaskRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.TaskRoleArn))
			assert.Equal(t, utility.FromStringPtr(opts.ExecutionRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ExecutionRoleArn))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions, 1)
			left, right := utility.StringSliceSymmetricDifference(containerDef.Command, utility.FromStringPtrSlice(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Command))
			assert.Empty(t, left)
			assert.Empty(t, right)
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets, 1)
			assert.Equal(t, utility.FromStringPtr(envVar.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets[0].Name))
			assert.Equal(t, utility.FromStringPtr(secretOpts.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets[0].ValueFrom))

			assert.Equal(t, utility.FromStringPtr(secretOpts.Name), utility.FromStringPtr(sm.CreateSecretInput.Name))
			assert.Equal(t, utility.FromStringPtr(secretOpts.Value), utility.FromStringPtr(sm.CreateSecretInput.SecretString))

			require.NotZero(t, c.RunTaskInput)
			require.Len(t, c.RunTaskInput.PlacementStrategy, 1)
			assert.EqualValues(t, *placementOpts.Strategy, utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Type))
			assert.Equal(t, utility.FromStringPtr(placementOpts.StrategyParameter), utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Field))
		},
	}
}
