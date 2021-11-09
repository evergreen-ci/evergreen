package testutil

import (
	"context"
	"os"
	"path"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

const projectName = "cocoa"

// NewSecretName creates a new test secret name with a common prefix, the given
// test's name, and a random string.
func NewSecretName(t *testing.T) string {
	return path.Join(secretName(t), utility.RandomString())
}

func secretName(t *testing.T) string {
	return path.Join(strings.TrimSuffix(SecretPrefix(), "/"), projectName, runtimeNamespace, t.Name())
}

// SecretPrefix returns the prefix name for secrets from the environment
// variable.
func SecretPrefix() string {
	return os.Getenv("AWS_SECRET_PREFIX")
}

// CleanupSecrets cleans up all existing secrets used in a test.
func CleanupSecrets(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient) {
	for token := cleanupSecretsWithToken(ctx, t, c, nil); token != nil; token = cleanupSecretsWithToken(ctx, t, c, token) {
	}
}

// cleanupSecretsWithToken cleans up existing secrets used in Cocoa tests based
// on the results from the pagination token.
func cleanupSecretsWithToken(ctx context.Context, t *testing.T, c cocoa.SecretsManagerClient, token *string) (nextToken *string) {
	out, err := c.ListSecrets(ctx, &secretsmanager.ListSecretsInput{
		NextToken: token,
	})
	if !assert.NoError(t, err) {
		return nil
	}
	if !assert.NotZero(t, out) {
		return nil
	}

	for _, secret := range out.SecretList {
		if secret == nil {
			continue
		}
		if secret.ARN == nil {
			continue
		}

		arn := *secret.ARN

		// Ignore secrets that were not generated within this test.
		name := secretName(t)
		if !strings.Contains(arn, name) {
			continue
		}

		_, err := c.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
			ForceDeleteWithoutRecovery: utility.TruePtr(),
			SecretId:                   &arn,
		})
		if assert.NoError(t, err) {
			grip.Info(message.Fields{
				"message": "cleaned up leftover secret",
				"arn":     arn,
				"test":    t.Name(),
			})
		}
	}

	return out.NextToken
}
