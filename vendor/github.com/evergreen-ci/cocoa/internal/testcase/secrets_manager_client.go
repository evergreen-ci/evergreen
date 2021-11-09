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
		"CreateSecretSucceeds": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(testutil.NewSecretName(t)),
				SecretString: aws.String(utility.RandomString()),
			})
			require.NoError(t, err)
			require.NotZero(t, out)

			cleanupSecret(ctx, t, c, out)
		},
		"CreateSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"GetSecretValueSucceedsWithExistingSecret": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			secretName := testutil.NewSecretName(t)
			createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(secretName),
				SecretString: aws.String("foo"),
			})
			require.NoError(t, err)
			require.NotZero(t, createOut)

			defer cleanupSecret(ctx, t, c, createOut)

			require.NotZero(t, createOut.ARN)

			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: createOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, out)
			assert.Equal(t, "foo", *out.SecretString)
			assert.Equal(t, secretName, *out.Name)
		},
		"GetSecretValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"GetSecretValueFailsWithValidNonexistentSecret": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: aws.String(testutil.NewSecretName(t)),
			})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"UpdateSecretValueSucceedsWithExistingSecret": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			secretName := testutil.NewSecretName(t)
			createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(secretName),
				SecretString: aws.String("bar"),
			})
			require.NoError(t, err)
			require.NotZero(t, createOut)

			defer cleanupSecret(ctx, t, c, createOut)

			require.NotZero(t, createOut.ARN)

			updateOut, err := c.UpdateSecretValue(ctx, &secretsmanager.UpdateSecretInput{
				SecretId:     createOut.ARN,
				SecretString: aws.String("leaf"),
			})
			require.NoError(t, err)
			require.NotZero(t, updateOut)

			getOut, err := c.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
				SecretId: createOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, getOut)
			assert.Equal(t, "leaf", utility.FromStringPtr(getOut.SecretString))
			assert.Equal(t, secretName, utility.FromStringPtr(getOut.Name))
		},
		"UpdateSecretValueFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.UpdateSecretValue(ctx, &secretsmanager.UpdateSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"UpdateSecretValueFailsWithValidNonexistentSecret": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.UpdateSecretValue(ctx, &secretsmanager.UpdateSecretInput{
				SecretId:     aws.String(testutil.NewSecretName(t)),
				SecretString: aws.String("hello"),
			})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"DescribeSecretSucceeds": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(testutil.NewSecretName(t)),
				SecretString: aws.String("bar"),
			})
			require.NoError(t, err)
			require.NotZero(t, createOut)

			defer cleanupSecret(ctx, t, c, createOut)

			require.NotZero(t, createOut.ARN)

			describeOut, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
				SecretId: createOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, describeOut)
			assert.Equal(t, createOut.ARN, describeOut.ARN)
		},
		"DescribeSecretSucceedsAfterDeletion": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(testutil.NewSecretName(t)),
				SecretString: aws.String("bar"),
			})
			require.NoError(t, err)
			require.NotZero(t, createOut)

			defer cleanupSecret(ctx, t, c, createOut)

			require.NotZero(t, createOut.ARN)

			cleanupSecret(ctx, t, c, createOut)

			describeOut, err := c.DescribeSecret(ctx, &secretsmanager.DescribeSecretInput{
				SecretId: createOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, describeOut)
			assert.Equal(t, createOut.ARN, describeOut.ARN)
		},
		"DescribeSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"DescribeSecretFailsWithValidNonexistentSecret": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
				SecretId: aws.String(testutil.NewSecretName(t)),
			})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"DeleteSecretFailsWithInvalidInput": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{})
			assert.Error(t, err)
			assert.Zero(t, out)
		},
		"DeleteSecretSucceeds": func(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
			createOut, err := c.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
				Name:         aws.String(testutil.NewSecretName(t)),
				SecretString: aws.String("hello"),
			})
			require.NoError(t, err)
			require.NotZero(t, createOut)
			out, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
				ForceDeleteWithoutRecovery: aws.Bool(true),
				SecretId:                   createOut.ARN,
			})
			require.NoError(t, err)
			require.NotZero(t, out)
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
