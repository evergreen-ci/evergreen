package units

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awsECS "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	evgMock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodDefinitionCleanupJob(t *testing.T) {
	defer func() {
		cocoaMock.ResetGlobalECSService()

		assert.NoError(t, db.ClearCollections(definition.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob){
		"WithExistingPodDefinition": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			for tName, tCase := range map[string]func(pd definition.PodDefinition){
				"CleansUpStaleUnusedPodDefinitions": func(pd definition.PodDefinition) {
					pd.LastAccessed = time.Now().Add(-9000 * 24 * time.Hour)
					require.NoError(t, pd.Upsert())

					j.Run(ctx)
					require.NoError(t, j.Error())

					describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
						TaskDefinition: aws.String(pd.ExternalID),
					})
					assert.NoError(t, err, "cloud pod definition should still exist even after it's been cleaned up")
					require.NotZero(t, describeOut.TaskDefinition)
					assert.Equal(t, awsECS.TaskDefinitionStatusInactive, utility.FromStringPtr(describeOut.TaskDefinition.Status), "cloud pod definition should be inactive")

					dbPodDef, err := definition.FindOneID(pd.ID)
					assert.NoError(t, err)
					assert.Zero(t, dbPodDef, "pod definition should have been cleaned up")
				},
				"NoopsWithoutAnyPodDefinitionsExceedingTTL": func(pd definition.PodDefinition) {
					assert.InDelta(t, time.Now().Unix(), pd.LastAccessed.Unix(), 1)

					j.Run(ctx)
					require.NoError(t, j.Error())

					describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
						TaskDefinition: aws.String(pd.ExternalID),
					})
					assert.NoError(t, err, "cloud pod definition should still exist")
					require.NotZero(t, describeOut.TaskDefinition)
					assert.Equal(t, awsECS.TaskDefinitionStatusActive, utility.FromStringPtr(describeOut.TaskDefinition.Status), "cloud pod definition should still be active")

					dbPodDef, err := definition.FindOneID(pd.ID)
					assert.NoError(t, err)
					assert.NotZero(t, dbPodDef, "pod definition should not have been cleaned up")
				},
				"ErrorsIfPodDefinitionCleanupErrors": func(pd definition.PodDefinition) {
					mockPodDefMgr, ok := j.podDefMgr.(*cocoaMock.ECSPodDefinitionManager)
					require.True(t, ok)
					mockPodDefMgr.DeletePodDefinitionError = errors.New("fake error")

					pd.LastAccessed = time.Now().Add(-9000 * 24 * time.Hour)
					require.NoError(t, pd.Upsert())

					j.Run(ctx)
					assert.Error(t, j.Error())

					describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
						TaskDefinition: aws.String(pd.ExternalID),
					})
					assert.NoError(t, err, "cloud pod definition should still exist")
					require.NotZero(t, describeOut.TaskDefinition)
					assert.Equal(t, awsECS.TaskDefinitionStatusActive, utility.FromStringPtr(describeOut.TaskDefinition.Status), "cloud pod definition should still be active")

					dbPodDef, err := definition.FindOneID(pd.ID)
					assert.NoError(t, err)
					assert.NotZero(t, dbPodDef, "pod definition should not have been cleaned up")
				},
			} {
				t.Run(tName, func(t *testing.T) {
					cocoaMock.ResetGlobalECSService()
					require.NoError(t, db.ClearCollections(definition.Collection))

					containerDef := cocoa.NewECSContainerDefinition().
						SetImage("image").
						SetCommand([]string{"echo", "hello"})
					podDefOpts := cocoa.NewECSPodDefinitionOptions().
						AddContainerDefinitions(*containerDef).
						SetCPU(128).
						SetMemoryMB(128)

					podDef, err := j.podDefMgr.CreatePodDefinition(ctx, *podDefOpts)
					require.NoError(t, err)

					describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
						TaskDefinition: aws.String(podDef.ID),
					})
					require.NoError(t, err, "cloud pod definition should have been created")
					require.NotZero(t, describeOut.TaskDefinition)
					assert.Equal(t, podDef.ID, utility.FromStringPtr(describeOut.TaskDefinition.TaskDefinitionArn))

					dbPodDef, err := definition.FindOneByExternalID(podDef.ID)
					require.NoError(t, err)
					require.NotZero(t, dbPodDef, "DB pod definition should have been created")

					tCase(*dbPodDef)
				})
			}
		},
		"NoopsWithoutAnyPodDefinitions": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			j.Run(ctx)
			assert.NoError(t, j.Error())
		},
		"CleansUpPodDefinitionMissingExternalID": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			pd := definition.PodDefinition{
				ID:           "pod_definition",
				LastAccessed: time.Now().Add(-1000 * 24 * time.Hour),
			}
			require.NoError(t, pd.Insert())

			j.Run(ctx)
			assert.NoError(t, j.Error())

			dbPodDef, err := definition.FindOneID(pd.ID)
			assert.NoError(t, err)
			assert.Zero(t, dbPodDef)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			cocoaMock.ResetGlobalECSService()

			require.NoError(t, db.ClearCollections(definition.Collection))

			env := &evgMock.Environment{}
			require.NoError(t, env.Configure(ctx))

			j, ok := NewPodDefinitionCleanupJob(utility.RoundPartOfMinute(0).Format(TSFormat)).(*podDefinitionCleanupJob)
			require.True(t, ok)

			j.env = env
			j.settings = *env.Settings()

			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(ctx))
			}()

			pdm, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, nil)
			require.NoError(t, err)
			j.podDefMgr = cocoaMock.NewECSPodDefinitionManager(pdm)

			tCase(ctx, t, j)
		})
	}
}
