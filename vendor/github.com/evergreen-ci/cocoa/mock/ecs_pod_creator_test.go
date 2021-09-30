package mock

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func cleanupECSAndSecretsManagerCache() {
	GlobalECSService.Clusters[testutil.ECSClusterName()] = ECSCluster{}
	GlobalSecretCache = map[string]StoredSecret{}
}

func TestECSPodCreator(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSPodCreator)(nil), &ECSPodCreator{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for tName, tCase := range ecsPodCreatorTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

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
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

			c := &ECSClient{}
			defer func() {
				assert.NoError(t, c.Close(ctx))
			}()

			pc, err := ecs.NewBasicECSPodCreator(c, nil)
			require.NoError(t, err)

			mpc := NewECSPodCreator(pc)

			tCase(tctx, t, mpc)
		})
	}

	for tName, tCase := range testcase.ECSPodCreatorWithVaultTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

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

			tCase(tctx, t, mpc)
		})
	}
}

// ecsPodCreatorTests are mock-specific tests for ECS and Secrets Manager with
// the ECS pod creator.
func ecsPodCreatorTests() map[string]func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
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
				SetGroup("group").
				SetStrategy(cocoa.StrategyBinpack).
				SetStrategyParameter(cocoa.StrategyParamBinpackMemory).
				AddInstanceFilters("runningTaskCount == 0", cocoa.ConstraintDistinctInstance)
			awsvpcOpts := cocoa.NewAWSVPCOptions().
				AddSubnets("subnet-12345").
				AddSecurityGroups("sg-12345")
			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName()).
				SetPlacementOptions(*placementOpts).
				SetAWSVPCOptions(*awsvpcOpts).
				SetTags(map[string]string{"execution_tag": "execution_val"}).
				SetSupportsDebugMode(true)
			opts := cocoa.NewECSPodCreationOptions().
				SetMemoryMB(512).
				SetCPU(1024).
				SetTaskRole("task_role").
				SetExecutionRole("execution_role").
				SetNetworkMode(cocoa.NetworkModeAWSVPC).
				SetTags(map[string]string{"creation_tag": "creation_val"}).
				AddContainerDefinitions(*containerDef).
				SetExecutionOptions(*execOpts)

			_, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			require.NotZero(t, c.RegisterTaskDefinitionInput)

			mem, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Memory))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.MemoryMB), mem)
			cpu, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Cpu))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.CPU), cpu)
			require.NotZero(t, opts.NetworkMode)
			assert.EqualValues(t, *opts.NetworkMode, utility.FromStringPtr(c.RegisterTaskDefinitionInput.NetworkMode))
			assert.Equal(t, utility.FromStringPtr(opts.TaskRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.TaskRoleArn))
			assert.Equal(t, utility.FromStringPtr(opts.ExecutionRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ExecutionRoleArn))
			require.Len(t, c.RegisterTaskDefinitionInput.Tags, 1)
			assert.Equal(t, "creation_tag", utility.FromStringPtr(c.RegisterTaskDefinitionInput.Tags[0].Key))
			assert.Equal(t, opts.Tags["creation_tag"], utility.FromStringPtr(c.RegisterTaskDefinitionInput.Tags[0].Value))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions, 1)
			assert.Equal(t, containerDef.Command, utility.FromStringPtrSlice(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Command))
			assert.Equal(t, utility.FromStringPtr(containerDef.WorkingDir), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].WorkingDirectory))
			assert.EqualValues(t, utility.FromIntPtr(containerDef.MemoryMB), utility.FromInt64Ptr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Memory))
			assert.EqualValues(t, utility.FromIntPtr(containerDef.CPU), utility.FromInt64Ptr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Cpu))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment, 1)
			assert.Equal(t, utility.FromStringPtr(envVar.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment[0].Name))
			assert.Equal(t, utility.FromStringPtr(envVar.Value), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Environment[0].Value))

			require.NotZero(t, c.RunTaskInput)
			assert.Equal(t, utility.FromStringPtr(execOpts.Cluster), utility.FromStringPtr(c.RunTaskInput.Cluster))
			assert.Equal(t, utility.FromStringPtr(placementOpts.Group), utility.FromStringPtr(c.RunTaskInput.Group))
			require.Len(t, c.RunTaskInput.PlacementStrategy, 1)
			assert.EqualValues(t, *placementOpts.Strategy, utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Type))
			assert.Equal(t, utility.FromStringPtr(placementOpts.StrategyParameter), utility.FromStringPtr(c.RunTaskInput.PlacementStrategy[0].Field))
			require.Len(t, c.RunTaskInput.PlacementConstraints, 2)
			assert.Equal(t, "memberOf", utility.FromStringPtr(c.RunTaskInput.PlacementConstraints[0].Type))
			assert.Equal(t, placementOpts.InstanceFilters[0], utility.FromStringPtr(c.RunTaskInput.PlacementConstraints[0].Expression))
			assert.Equal(t, cocoa.ConstraintDistinctInstance, utility.FromStringPtr(c.RunTaskInput.PlacementConstraints[1].Type))
			assert.Zero(t, c.RunTaskInput.PlacementConstraints[1].Expression)
			require.NotZero(t, c.RunTaskInput.NetworkConfiguration)
			require.NotZero(t, c.RunTaskInput.NetworkConfiguration.AwsvpcConfiguration)
			assert.ElementsMatch(t, execOpts.AWSVPCOpts.Subnets, utility.FromStringPtrSlice(c.RunTaskInput.NetworkConfiguration.AwsvpcConfiguration.Subnets))
			assert.ElementsMatch(t, execOpts.AWSVPCOpts.SecurityGroups, utility.FromStringPtrSlice(c.RunTaskInput.NetworkConfiguration.AwsvpcConfiguration.SecurityGroups))
			require.Len(t, c.RunTaskInput.Tags, 1)
			assert.Equal(t, "execution_tag", utility.FromStringPtr(c.RunTaskInput.Tags[0].Key))
			assert.Equal(t, execOpts.Tags["execution_tag"], utility.FromStringPtr(c.RunTaskInput.Tags[0].Value))
			assert.True(t, utility.FromBoolPtr(c.RunTaskInput.EnableExecuteCommand))
		},
		"RegistersTaskDefinitionAndRunsTaskWithCreatedSecrets": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
			secretOpts := cocoa.NewSecretOptions().
				SetName("secret_name").
				SetNewValue("secret_value")
			envVar := cocoa.NewEnvironmentVariable().
				SetName("env_var_name").
				SetSecretOptions(*secretOpts)
			containerDef := cocoa.NewECSContainerDefinition().
				SetName("name").
				SetImage("image").
				SetCommand([]string{"echo", "foo"}).
				AddEnvironmentVariables(*envVar)
			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName())
			opts := cocoa.NewECSPodCreationOptions().
				SetMemoryMB(512).
				SetCPU(1024).
				SetExecutionRole("execution_role").
				AddContainerDefinitions(*containerDef).
				SetExecutionOptions(*execOpts)

			_, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			require.NotZero(t, c.RegisterTaskDefinitionInput)

			mem, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Memory))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.MemoryMB), mem)
			cpu, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Cpu))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.CPU), cpu)
			assert.Equal(t, utility.FromStringPtr(opts.ExecutionRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ExecutionRoleArn))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions, 1)
			assert.Equal(t, containerDef.Command, utility.FromStringPtrSlice(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Command))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets, 1)
			assert.Equal(t, utility.FromStringPtr(envVar.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets[0].Name))
			assert.Equal(t, utility.FromStringPtr(secretOpts.Name), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Secrets[0].ValueFrom))

			assert.Equal(t, utility.FromStringPtr(secretOpts.Name), utility.FromStringPtr(sm.CreateSecretInput.Name))
			assert.Equal(t, utility.FromStringPtr(secretOpts.NewValue), utility.FromStringPtr(sm.CreateSecretInput.SecretString))
		},
		"RegistersTaskDefinitionAndRunsTaskWithCreatedRepositoryCredentials": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
			repoCreds := cocoa.NewRepositoryCredentials().
				SetName("repo_creds_secret_name").
				SetNewCredentials(*cocoa.NewStoredRepositoryCredentials().
					SetUsername("username").
					SetPassword("password"))
			containerDef := cocoa.NewECSContainerDefinition().
				SetName("name").
				SetImage("image").
				SetCommand([]string{"echo", "foo"}).
				SetRepositoryCredentials(*repoCreds)
			execOpts := cocoa.NewECSPodExecutionOptions().
				SetCluster(testutil.ECSClusterName())
			opts := cocoa.NewECSPodCreationOptions().
				SetMemoryMB(512).
				SetCPU(1024).
				SetExecutionRole("execution_role").
				AddContainerDefinitions(*containerDef).
				SetExecutionOptions(*execOpts)

			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			require.NotZero(t, c.RegisterTaskDefinitionInput)

			mem, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Memory))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.MemoryMB), mem)
			cpu, err := strconv.Atoi(utility.FromStringPtr(c.RegisterTaskDefinitionInput.Cpu))
			require.NoError(t, err)
			assert.Equal(t, utility.FromIntPtr(opts.CPU), cpu)
			assert.Equal(t, utility.FromStringPtr(opts.ExecutionRole), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ExecutionRoleArn))
			require.Len(t, c.RegisterTaskDefinitionInput.ContainerDefinitions, 1)
			assert.Equal(t, containerDef.Command, utility.FromStringPtrSlice(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].Command))
			res := p.Resources()
			require.Len(t, res.Containers, 1)
			require.Len(t, res.Containers[0].Secrets, 1)
			require.NotZero(t, c.RegisterTaskDefinitionInput.ContainerDefinitions[0].RepositoryCredentials, 1)
			assert.Equal(t, utility.FromStringPtr(res.Containers[0].Secrets[0].ID), utility.FromStringPtr(c.RegisterTaskDefinitionInput.ContainerDefinitions[0].RepositoryCredentials.CredentialsParameter))

			assert.Equal(t, utility.FromStringPtr(repoCreds.Name), utility.FromStringPtr(sm.CreateSecretInput.Name))
			storedCreds, err := json.Marshal(repoCreds.NewCreds)
			require.NoError(t, err)
			assert.Equal(t, string(storedCreds), utility.FromStringPtr(sm.CreateSecretInput.SecretString))
		},
		"CreatingNewSecretsIsRetryable": func(ctx context.Context, t *testing.T, pc cocoa.ECSPodCreator, c *ECSClient, sm *SecretsManagerClient) {
			secretOpts := cocoa.NewSecretOptions().
				SetName("secret_name").
				SetNewValue("secret_value")
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

			c.RegisterTaskDefinitionError = errors.New("fake error")
			c.RunTaskError = errors.New("fake error")

			_, err := pc.CreatePod(ctx, *opts)
			require.Error(t, err)

			secret, ok := GlobalSecretCache[utility.FromStringPtr(secretOpts.Name)]
			require.True(t, ok)
			assert.Equal(t, utility.FromStringPtr(secretOpts.NewValue), secret.Value)

			c.RegisterTaskDefinitionError = nil
			c.RunTaskError = nil

			p, err := pc.CreatePod(ctx, *opts)
			require.NoError(t, err)

			require.Len(t, p.Resources().Containers, 1)
			require.Len(t, p.Resources().Containers[0].Secrets, 1)

			getSecretOut, err := sm.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{SecretId: p.Resources().Containers[0].Secrets[0].ID})
			require.NoError(t, err)
			require.NotZero(t, getSecretOut)
			assert.Equal(t, utility.FromStringPtr(secretOpts.NewValue), utility.FromStringPtr(getSecretOut.SecretString))
		},
	}
}
