package cocoa

import (
	"context"

	"github.com/aws/aws-sdk-go/service/secretsmanager"
)

// SecretsManagerClient provides a common interface to interact with a client
// backed by AWS Secrets Manager. Implementations must handle retrying and
// backoff.
type SecretsManagerClient interface {
	// CreateSecret creates a new secret.
	CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error)
	// GetSecretValue gets the decrypted value of a secret.
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error)
	// DescribeSecret gets metadata information about a secret.
	DescribeSecret(ctx context.Context, in *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error)
	// ListSecrets lists all metadata information for secrets matching the
	// filters.
	ListSecrets(ctx context.Context, in *secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error)
	// UpdateSecret updates the value of an existing secret.
	UpdateSecretValue(ctx context.Context, in *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error)
	// DeleteSecret deletes an existing secret.
	DeleteSecret(ctx context.Context, in *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error)
	// Close closes the client and cleans up its resources. Implementations
	// should ensure that this is idempotent.
	Close(ctx context.Context) error
}
