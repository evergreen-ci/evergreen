package units

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	secretsmanagerTypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	cocoaMock "github.com/evergreen-ci/cocoa/mock"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerSecretCleanupJob(t *testing.T) {
	defer cocoaMock.ResetGlobalSecretCache()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, j *containerSecretCleanupJob){
		"DeletesStrandedSecretsWithMatchingTag": func(ctx context.Context, t *testing.T, j *containerSecretCleanupJob) {
			var secretIDs []string
			for i := 0; i < 5; i++ {
				createOut, err := j.smClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
					Name:         aws.String(fmt.Sprintf("secret_name%d", i)),
					SecretString: aws.String("secret_string"),
					Tags: []secretsmanagerTypes.Tag{{
						Key:   aws.String(model.ContainerSecretTag),
						Value: aws.String(strconv.FormatBool(false)),
					}},
				})
				require.NoError(t, err)
				secretIDs = append(secretIDs, utility.FromStringPtr(createOut.ARN))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			for _, secretID := range secretIDs {
				val, err := j.vault.GetValue(ctx, secretID)
				assert.Error(t, err, "secret should have been deleted")
				assert.Zero(t, val)
			}
		},
		"DeletesLimitedNumberOfStrandedSecrets": func(ctx context.Context, t *testing.T, j *containerSecretCleanupJob) {
			mockEnv, ok := j.env.(*mock.Environment)
			require.True(t, ok)
			const cleanupLimit = 2
			mockEnv.EvergreenSettings.PodLifecycle.MaxSecretCleanupRate = cleanupLimit

			var secretIDs []string
			for i := 0; i < 5; i++ {
				createOut, err := j.smClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
					Name:         aws.String(fmt.Sprintf("secret_name%d", i)),
					SecretString: aws.String("secret_string"),
					Tags: []secretsmanagerTypes.Tag{{
						Key:   aws.String(model.ContainerSecretTag),
						Value: aws.String(strconv.FormatBool(false)),
					}},
				})
				require.NoError(t, err)
				secretIDs = append(secretIDs, utility.FromStringPtr(createOut.ARN))
			}

			j.Run(ctx)
			assert.NoError(t, j.Error())

			var numDeleted int
			for _, secretID := range secretIDs {
				if _, err := j.vault.GetValue(ctx, secretID); err != nil {
					numDeleted++
				}
			}
			assert.Equal(t, cleanupLimit, numDeleted)
		},
		"NoopsWithNoSecrets": func(ctx context.Context, t *testing.T, j *containerSecretCleanupJob) {
			j.Run(ctx)
			assert.NoError(t, j.Error())
		},
		"NoopsWithNoSecretsMatchingTag": func(ctx context.Context, t *testing.T, j *containerSecretCleanupJob) {
			createOut, err := j.smClient.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String("secret_name"),
				SecretString: aws.String("secret_string"),
				Tags:         []secretsmanagerTypes.Tag{{Key: aws.String("cherry"), Value: aws.String("tomato")}},
			})
			require.NoError(t, err)

			j.Run(ctx)
			assert.NoError(t, j.Error())

			val, err := j.vault.GetValue(ctx, utility.FromStringPtr(createOut.ARN))
			assert.NoError(t, err, "secret should still exist")
			assert.NotZero(t, val)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx := testutil.TestSpan(ctx, t)

			cocoaMock.ResetGlobalSecretCache()

			j, ok := NewContainerSecretCleanupJob(utility.RoundPartOfHour(0).Format(TSFormat)).(*containerSecretCleanupJob)
			require.True(t, ok)
			j.tagClient = &cocoaMock.TagClient{}
			j.smClient = &cocoaMock.SecretsManagerClient{}
			v, err := secret.NewBasicSecretsManager(*secret.NewBasicSecretsManagerOptions().
				SetClient(j.smClient).
				SetCache(&cloud.NoopSecretCache{Tag: model.ContainerSecretTag}))
			require.NoError(t, err)
			j.vault = cocoaMock.NewVault(v)

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			env.EvergreenSettings.PodLifecycle.MaxSecretCleanupRate = 1000
			j.env = env

			tCase(tctx, t, j)
		})
	}
}
