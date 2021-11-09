package secret

import (
	"context"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/utility"
)

// BasicSecretsManagerClient provides a cocoa.SecretsManagerClient
// implementation that wraps the AWS Secrets Manager API. It supports
// retrying requests using exponential backoff and jitter.
type BasicSecretsManagerClient struct {
	sm      *secretsmanager.SecretsManager
	opts    awsutil.ClientOptions
	session *session.Session
}

// NewBasicSecretsManagerClient creates a new AWS Secrets Manager client from
// the given options.
func NewBasicSecretsManagerClient(opts awsutil.ClientOptions) (*BasicSecretsManagerClient, error) {
	c := &BasicSecretsManagerClient{
		opts: opts,
	}
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	return c, nil
}

func (c *BasicSecretsManagerClient) setup() error {
	if err := c.opts.Validate(); err != nil {
		return errors.Wrap(err, "invalid options")
	}

	if c.sm != nil {
		return nil
	}

	if err := c.setupSession(); err != nil {
		return errors.Wrap(err, "setting up session")
	}

	c.sm = secretsmanager.New(c.session)

	return nil
}

func (c *BasicSecretsManagerClient) setupSession() error {
	if c.session != nil {
		return nil
	}

	creds, err := c.opts.GetCredentials()
	if err != nil {
		return errors.Wrap(err, "getting credentials")
	}
	sess, err := session.NewSession(&aws.Config{
		HTTPClient:  c.opts.HTTPClient,
		Region:      c.opts.Region,
		Credentials: creds,
	})
	if err != nil {
		return errors.Wrap(err, "creating session")
	}

	c.session = sess

	return nil
}

// CreateSecret creates a new secret.
func (c *BasicSecretsManagerClient) CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.CreateSecretOutput
	var err error
	msg := awsutil.MakeAPILogMessage("CreateSecret", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.CreateSecretWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}
	return out, nil
}

// GetSecretValue gets the decrypted value of an existing secret.
func (c *BasicSecretsManagerClient) GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.GetSecretValueOutput
	var err error
	msg := awsutil.MakeAPILogMessage("GetSecretValue", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.GetSecretValueWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}
	return out, nil
}

// DescribeSecret gets the metadata information about a secret.
func (c *BasicSecretsManagerClient) DescribeSecret(ctx context.Context, in *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.DescribeSecretOutput
	var err error
	msg := awsutil.MakeAPILogMessage("DescribeSecret", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.DescribeSecretWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}

	return out, nil
}

// ListSecrets lists the metadata information for secrets matching the filters.
func (c *BasicSecretsManagerClient) ListSecrets(ctx context.Context, in *secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.ListSecretsOutput
	var err error
	msg := awsutil.MakeAPILogMessage("ListSecrets", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.ListSecretsWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}

	return out, nil
}

// UpdateSecretValue updates the value of an existing secret.
func (c *BasicSecretsManagerClient) UpdateSecretValue(ctx context.Context, in *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.UpdateSecretOutput
	var err error
	msg := awsutil.MakeAPILogMessage("UpdateSecret", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.UpdateSecretWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}
	return out, nil
}

// DeleteSecret deletes an existing secret.
func (c *BasicSecretsManagerClient) DeleteSecret(ctx context.Context, in *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
	if err := c.setup(); err != nil {
		return nil, errors.Wrap(err, "setting up client")
	}

	var out *secretsmanager.DeleteSecretOutput
	var err error
	msg := awsutil.MakeAPILogMessage("DeleteSecret", in)
	if err := utility.Retry(
		ctx,
		func() (bool, error) {
			out, err = c.sm.DeleteSecretWithContext(ctx, in)
			if awsErr, ok := err.(awserr.Error); ok {
				grip.Debug(message.WrapError(awsErr, msg))
				if c.isNonRetryableErrorCode(awsErr.Code()) {
					return false, err
				}
			}
			return true, err
		}, *c.opts.RetryOpts); err != nil {
		return nil, err
	}
	return out, nil
}

// Close closes the client.
func (c *BasicSecretsManagerClient) Close(ctx context.Context) error {
	c.opts.Close()
	return nil
}

// isNonRetryableErrorCode returns whether or not the error code from Secrets
// Manager is known to be not retryable.
func (c *BasicSecretsManagerClient) isNonRetryableErrorCode(code string) bool {
	switch code {
	case "AccessDeniedException",
		secretsmanager.ErrCodeInvalidParameterException,
		secretsmanager.ErrCodeInvalidRequestException,
		secretsmanager.ErrCodeResourceNotFoundException,
		secretsmanager.ErrCodeResourceExistsException,
		request.InvalidParameterErrCode,
		request.ParamRequiredErrCode:
		return true
	default:
		return false
	}
}
