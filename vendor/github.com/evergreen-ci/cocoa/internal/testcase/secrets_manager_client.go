package testcase

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// SecretsManagerClientTestCase represents a test case for a
// cocoa.SecretsManagerClient.
type SecretsManagerClientTestCase func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient)

// SecretsManagerClientTests returns common test cases that a
// cocoa.SecretsManagerClient should support.
func SecretsManagerClientTests() map[string]SecretsManagerClientTestCase {
	return map[string]SecretsManagerClientTestCase{
		"CreateSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"DeleteSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"CreateAndDeleteSecretSucceed": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			createSecretOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(testutil.NewSecretName(t.Name())),
				SecretString: aws.String("hello"),
			})
			require.NoError(t, err)
			require.NotZero(t, createSecretOut)
			require.NotZero(t, createSecretOut.ARN)
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
				ForceDeleteWithoutRecovery: aws.Bool(true),
				SecretId:                   createSecretOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, out)

		},
		"GetValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"GetValueFailsWithValidNonexistentInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(utility.RandomString()),
			})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"UpdateValueFailsWithValidNonexistentInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.UpdateSecretValue(ctx, &secretsmanager.UpdateSecretInput{
				SecretId:     aws.String(utility.RandomString()),
				SecretString: aws.String("hello"),
			})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"CreateAndGetSucceed": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			secretName := testutil.NewSecretName(t.Name())
			outCreate, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(secretName),
				SecretString: aws.String("foo"),
			})
			require.NoError(t, err)
			require.NotZero(t, outCreate)
			require.NotZero(t, &outCreate)

			defer cleanupSecret(ctx, t, c, outCreate)

			require.NotZero(t, outCreate.ARN)
			require.NotZero(t, &outCreate.ARN)

			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: outCreate.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, out)
			assert.Equal(t, "foo", *out.SecretString)
			assert.Equal(t, secretName, *out.Name)
		},
		"UpdateSecretModifiesValue": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			secretName := testutil.NewSecretName(t.Name())
			out, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(secretName),
				SecretString: aws.String("bar"),
			})
			require.NoError(t, err)
			require.NotZero(t, out)
			require.NotZero(t, &out)

			defer cleanupSecret(ctx, t, c, out)

			require.NotZero(t, out.ARN)

			if out != nil && out.ARN != nil {
				out, err := c.UpdateSecretValue(ctx, &secretsmanager.UpdateSecretInput{
					SecretId:     out.ARN,
					SecretString: aws.String("leaf"),
				})
				require.NoError(t, err)
				require.NotZero(t, out)
			}

			if out != nil && out.ARN != nil {
				out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
					SecretId: out.ARN,
				})
				require.NoError(t, err)
				require.NotZero(t, out)
				assert.Equal(t, "leaf", *out.SecretString)
				assert.Equal(t, secretName, *out.Name)
			}
		},
	}
}

// cleanupSecret cleans up an existing secret if it exists.
func cleanupSecret(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient, out *secretsmanager.CreateSecretOutput) {
	if out != nil && out.ARN != nil {
		out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			ForceDeleteWithoutRecovery: aws.Bool(true),
			SecretId:                   out.ARN,
		})
		require.NoError(t, err)
		require.NotZero(t, out)
	}
}
