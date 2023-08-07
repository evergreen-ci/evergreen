package units

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	awsECS "github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	evgMock "github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodDefinitionCleanupJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		cocoaMock.ResetGlobalECSService()

		assert.NoError(t, db.ClearCollections(definition.Collection))
	}()

	createPodDef := func(ctx context.Context, t *testing.T, podDefMgr cocoa.ECSPodDefinitionManager, ecsClient cocoa.ECSClient) definition.PodDefinition {
		containerDef := cocoa.NewECSContainerDefinition().
			SetImage("image").
			SetCommand([]string{"echo", "hello"})
		podDefOpts := cocoa.NewECSPodDefinitionOptions().
			AddContainerDefinitions(*containerDef).
			SetCPU(128).
			SetMemoryMB(128)

		podDef, err := podDefMgr.CreatePodDefinition(ctx, *podDefOpts)
		require.NoError(t, err)

		describeOut, err := ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
			TaskDefinition: aws.String(podDef.ID),
		})
		require.NoError(t, err, "cloud pod definition should have been created")
		require.NotZero(t, describeOut.TaskDefinition)
		assert.Equal(t, podDef.ID, utility.FromStringPtr(describeOut.TaskDefinition.TaskDefinitionArn))

		dbPodDef, err := definition.FindOneByExternalID(podDef.ID)
		require.NoError(t, err)
		require.NotZero(t, dbPodDef, "DB pod definition should have been created")

		return *dbPodDef
	}
	createStrandedPodDef := func(ctx context.Context, t *testing.T, ecsClient cocoa.ECSClient, tags []*awsECS.Tag) awsECS.TaskDefinition {
		registerIn := awsECS.RegisterTaskDefinitionInput{
			Family: aws.String("family"),
			ContainerDefinitions: []*awsECS.ContainerDefinition{
				{
					Image:   aws.String("image"),
					Command: []*string{aws.String("echo"), aws.String("hello")},
				},
			},
			Tags: tags,
		}

		resp, err := ecsClient.RegisterTaskDefinition(ctx, &registerIn)
		require.NoError(t, err)
		require.NotZero(t, resp.TaskDefinition)

		return *resp.TaskDefinition
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob){
		"DeletesStrandedPodDefinitionsWithMatchingTag": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			var podDefIDs []string
			for i := 0; i < 5; i++ {
				def := createStrandedPodDef(ctx, t, j.ecsClient, []*awsECS.Tag{{Key: aws.String(definition.PodDefinitionTag), Value: aws.String(strconv.FormatBool(false))}})
				podDefIDs = append(podDefIDs, utility.FromStringPtr(def.TaskDefinitionArn))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			for _, podDefID := range podDefIDs {
				describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
					TaskDefinition: aws.String(podDefID),
				})
				require.NoError(t, err)
				assert.Equal(t, awsECS.TaskDefinitionStatusInactive, utility.FromStringPtr(describeOut.TaskDefinition.Status))
			}
		},
		"DeletesLimitedNumberOfStrandedPodDefinitions": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			mockEnv, ok := j.env.(*mock.Environment)
			require.True(t, ok)
			const cleanupLimit = 2
			mockEnv.EvergreenSettings.PodLifecycle.MaxPodDefinitionCleanupRate = cleanupLimit

			var podDefIDs []string
			for i := 0; i < 5; i++ {
				def := createStrandedPodDef(ctx, t, j.ecsClient, []*awsECS.Tag{{Key: aws.String(definition.PodDefinitionTag), Value: aws.String(strconv.FormatBool(false))}})
				podDefIDs = append(podDefIDs, utility.FromStringPtr(def.TaskDefinitionArn))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			var numDeleted int
			for _, podDefID := range podDefIDs {
				describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
					TaskDefinition: aws.String(podDefID),
				})
				require.NoError(t, err)
				if utility.FromStringPtr(describeOut.TaskDefinition.Status) == awsECS.TaskDefinitionStatusInactive {
					numDeleted++
				}
			}

			assert.Equal(t, numDeleted, cleanupLimit)
		},
		"NoopsWithNoPodDefinitionsMatchingStrandedMarkerTag": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			def := createStrandedPodDef(ctx, t, j.ecsClient, nil)

			j.Run(ctx)
			assert.NoError(t, j.Error())

			describeOut, err := j.ecsClient.DescribeTaskDefinition(ctx, &awsECS.DescribeTaskDefinitionInput{
				TaskDefinition: def.TaskDefinitionArn,
			})
			require.NoError(t, err)
			assert.Equal(t, awsECS.TaskDefinitionStatusActive, utility.FromStringPtr(describeOut.TaskDefinition.Status))
		},
		"CleansUpStaleUnusedPodDefinitions": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			pd := createPodDef(ctx, t, j.podDefMgr, j.ecsClient)
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
		"NoopsWithoutAnyPodDefinitionsExceedingTTL": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			pd := createPodDef(ctx, t, j.podDefMgr, j.ecsClient)
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
		"ErrorsIfPodDefinitionCleanupErrors": func(ctx context.Context, t *testing.T, j *podDefinitionCleanupJob) {
			mockPodDefMgr, ok := j.podDefMgr.(*cocoaMock.ECSPodDefinitionManager)
			require.True(t, ok)
			mockPodDefMgr.DeletePodDefinitionError = errors.New("fake error")
			defer func() {
				mockPodDefMgr.DeletePodDefinitionError = nil
			}()

			pd := createPodDef(ctx, t, j.podDefMgr, j.ecsClient)
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
			tctx, cancel := context.WithCancel(ctx)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

			cocoaMock.ResetGlobalECSService()

			require.NoError(t, db.ClearCollections(definition.Collection))

			env := &evgMock.Environment{}
			require.NoError(t, env.Configure(tctx))
			env.EvergreenSettings.PodLifecycle.MaxPodDefinitionCleanupRate = 1000

			j, ok := NewPodDefinitionCleanupJob(utility.RoundPartOfMinute(0).Format(TSFormat)).(*podDefinitionCleanupJob)
			require.True(t, ok)

			j.env = env
			j.settings = *env.Settings()

			j.ecsClient = &cocoaMock.ECSClient{}
			defer func() {
				assert.NoError(t, j.ecsClient.Close(tctx))
			}()
			j.tagClient = &cocoaMock.TagClient{}
			defer func() {
				assert.NoError(t, j.tagClient.Close(tctx))
			}()

			pdm, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, nil)
			require.NoError(t, err)
			j.podDefMgr = cocoaMock.NewECSPodDefinitionManager(pdm)

			tCase(tctx, t, j)
		})
	}
}
