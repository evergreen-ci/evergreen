package units

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	evgMock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewPodDefinitionCreationJob(t *testing.T) {
	opts := pod.TaskContainerCreationOptions{
		Image:      "image",
		MemoryMB:   512,
		CPU:        256,
		WorkingDir: "/working_dir",
	}
	ecsConf := evergreen.ECSConfig{
		TaskDefinitionPrefix: "prefix",
	}
	j, ok := NewPodDefinitionCreationJob(ecsConf, opts, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podDefinitionCreationJob)
	require.True(t, ok)
	assert.NotZero(t, j.ID())
	assert.Equal(t, opts, j.ContainerOpts)
	assert.Equal(t, opts.GetFamily(ecsConf), j.Family)
}

func TestPodDefinitionCreationJob(t *testing.T) {
	defer func() {
		cocoaMock.ResetGlobalECSService()
		cocoaMock.ResetGlobalSecretCache()
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podDefinitionCreationJob, p *pod.Pod){
		"Succeeds": func(ctx context.Context, t *testing.T, j *podDefinitionCreationJob, p *pod.Pod) {
			require.NoError(t, p.Insert())

			j.Run(ctx)
			require.NoError(t, j.Error())

			podDef, err := definition.FindOne(db.Query(bson.M{}))
			require.NoError(t, err)
			require.NotZero(t, podDef, "pod definition creation should have inserted a document")
			assert.Equal(t, j.Family, podDef.Family)
			assert.NotZero(t, podDef.ExternalID)
			assert.NotZero(t, podDef.LastAccessed)

			describeResp, err := j.ecsClient.DescribeTaskDefinition(ctx, &ecs.DescribeTaskDefinitionInput{
				Include:        []*string{aws.String("TAGS")},
				TaskDefinition: aws.String(podDef.ExternalID),
			})
			require.NoError(t, err)
			require.NotZero(t, describeResp)
			var cacheTagFound bool
			for _, tag := range describeResp.Tags {
				if utility.FromStringPtr(tag.Key) == cloud.PodDefinitionTag {
					assert.Equal(t, "true", utility.FromStringPtr(tag.Value))
					cacheTagFound = true
					break
				}
			}
			assert.True(t, cacheTagFound, "ECS task definition should have tag")

			taskDef := describeResp.TaskDefinition
			podConf := j.settings.Providers.AWS.Pod

			assert.True(t, strings.HasPrefix(utility.FromStringPtr(taskDef.Family), podConf.ECS.TaskDefinitionPrefix))
			assert.Equal(t, fmt.Sprint(j.ContainerOpts.CPU), utility.FromStringPtr(taskDef.Cpu))
			assert.Equal(t, fmt.Sprint(j.ContainerOpts.MemoryMB), utility.FromStringPtr(taskDef.Memory))
			// kim: TODO: upgrade Cocoa to include the execution role in the
			// mock ECS service task def.
			assert.Equal(t, podConf.ECS.ExecutionRole, utility.FromStringPtr(taskDef.ExecutionRoleArn))
			assert.Equal(t, podConf.ECS.TaskRole, utility.FromStringPtr(taskDef.TaskRoleArn))

			require.Len(t, taskDef.ContainerDefinitions, 1)
			containerDef := taskDef.ContainerDefinitions[0]
			assert.NotZero(t, containerDef.Command)
			assert.Equal(t, j.ContainerOpts.Image, utility.FromStringPtr(containerDef.Image))

			assert.Len(t, containerDef.Environment, len(j.ContainerOpts.EnvVars))
			for _, envVar := range containerDef.Environment {
				name := utility.FromStringPtr(envVar.Name)
				expectedVal, ok := j.ContainerOpts.EnvVars[name]
				if assert.True(t, ok, "unexpected environment variable '%s'", name) {
					assert.Equal(t, expectedVal, utility.FromStringPtr(envVar.Value), "incorrect value for environment variable '%s'", name)
				}
			}

			assert.Len(t, containerDef.Secrets, len(j.ContainerOpts.EnvSecrets))
			for _, secret := range containerDef.Secrets {
				name := utility.FromStringPtr(secret.Name)
				expectedSecret, ok := j.ContainerOpts.EnvSecrets[name]
				require.True(t, ok, "unexpected environment secret '%s'", name)
				assert.Contains(t, name, expectedSecret.Name)

				secretRef := utility.FromStringPtr(secret.ValueFrom)
				assert.True(t, strings.HasPrefix(secretRef, podConf.SecretsManager.SecretPrefix), "created secrets should include the secret prefix")

				val, err := j.vault.GetValue(ctx, secretRef)
				require.NoError(t, err, "missing expected secret '%s'", name)

				assert.Equal(t, expectedSecret.Value, val, "incorrect value for secret '%s'", name)
			}
		},
		"NoopsWithAlreadyExistingPodDefinition": func(ctx context.Context, t *testing.T, j *podDefinitionCreationJob, p *pod.Pod) {
			require.NoError(t, p.Insert())

			require.NoError(t, db.Insert(definition.Collection, definition.PodDefinition{
				ID:         utility.RandomString(),
				Family:     j.Family,
				ExternalID: "external_id",
			}))

			j.Run(ctx)
			require.NoError(t, j.Error())

			podDef, err := definition.FindOne(db.Query(bson.M{}))
			require.NoError(t, err)
			require.NotZero(t, podDef, "pre-existing pod definition should still exist")
			assert.Equal(t, j.Family, podDef.Family)

			pdm, ok := j.ecsPodDefManager.(*cocoaMock.ECSPodDefinitionManager)
			require.True(t, ok)
			assert.Zero(t, pdm.CreatePodDefinitionInput, "should not have created a pod definition")
		},
		"NoopsWithNoDependentIntentPods": func(ctx context.Context, t *testing.T, j *podDefinitionCreationJob, p *pod.Pod) {
			j.Run(ctx)
			require.NoError(t, j.Error())

			podDef, err := definition.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, podDef, "should not have cached a pod definition")

			pdm, ok := j.ecsPodDefManager.(*cocoaMock.ECSPodDefinitionManager)
			require.True(t, ok)
			assert.Zero(t, pdm.CreatePodDefinitionInput, "should not have created a pod definition")

			assert.Empty(t, cocoaMock.GlobalECSService.TaskDefs, "should not have created an ECS task definition")
			assert.Empty(t, cocoaMock.GlobalSecretCache, "should not have created any secrets")
		},
		"DecommissionsDependentIntentPodsWithNoRetriesRemaining": func(ctx context.Context, t *testing.T, j *podDefinitionCreationJob, p *pod.Pod) {
			require.NoError(t, p.Insert())

			pdm, ok := j.ecsPodDefManager.(*cocoaMock.ECSPodDefinitionManager)
			require.True(t, ok)
			pdm.CreatePodDefinitionError = errors.New("fail")

			j.UpdateRetryInfo(amboy.JobRetryOptions{
				CurrentAttempt: utility.ToIntPtr(j.RetryInfo().GetMaxAttempts()),
			})

			j.Run(ctx)
			assert.Error(t, j.Error(), "job should have errored due to empty container options")

			podDef, err := definition.FindOne(db.Query(bson.M{}))
			assert.NoError(t, err)
			assert.Zero(t, podDef, "should not have cached a pod definition")

			assert.Empty(t, cocoaMock.GlobalECSService.TaskDefs, "should not have created an ECS task definition")
			assert.Empty(t, cocoaMock.GlobalSecretCache, "should not have created any secrets")

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Equal(t, pod.StatusDecommissioned, dbPod.Status, "intent pod should have been decommissioned after pod definition creation failed")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(definition.Collection, pod.Collection, event.LegacyEventLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(definition.Collection, pod.Collection, event.LegacyEventLogCollection))
			}()

			cocoaMock.ResetGlobalECSService()
			cocoaMock.ResetGlobalSecretCache()

			env := &evgMock.Environment{}
			require.NoError(t, env.Configure(ctx))
			env.EvergreenSettings.Providers.AWS.Pod.ECS = evergreen.ECSConfig{
				TaskDefinitionPrefix: "task_definition_prefix",
				TaskRole:             "task_role",
				ExecutionRole:        "execution_role",
			}
			env.EvergreenSettings.Providers.AWS.Pod.SecretsManager = evergreen.SecretsManagerConfig{
				SecretPrefix: "secret-prefix",
			}
			ecsConf := env.EvergreenSettings.Providers.AWS.Pod.ECS

			p, err := pod.NewTaskIntentPod(ecsConf, pod.TaskIntentPodOptions{
				CPU:        128,
				MemoryMB:   256,
				OS:         pod.OSLinux,
				Arch:       pod.ArchAMD64,
				Image:      "ubuntu",
				WorkingDir: "/working_dir",
			})
			require.NoError(t, err)

			j, ok := NewPodDefinitionCreationJob(ecsConf, p.TaskContainerCreationOpts, utility.RoundPartOfMinute(0).Format(TSFormat)).(*podDefinitionCreationJob)
			require.True(t, ok)

			j.env = env
			j.settings = *env.Settings()

			j.smClient = &cocoaMock.SecretsManagerClient{}
			defer func() {
				assert.NoError(t, j.smClient.Close(ctx))
			}()

			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(ctx))
			}()

			j.vault = cocoaMock.NewVault(cloud.MakeSecretsManagerVault(j.smClient))

			pdm, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, j.vault)
			require.NoError(t, err)
			j.ecsPodDefManager = cocoaMock.NewECSPodDefinitionManager(pdm)

			tCase(ctx, t, j, p)
		})
	}
}
