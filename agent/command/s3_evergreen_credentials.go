package command

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/pkg/errors"
)

// evergreenCredentialProvider is an AWS credential provider that
// retrieves credentials from Evergreen.
type evergreenCredentialProvider struct {
	comm     client.Communicator
	taskData client.TaskData

	// roleARN is the ARN of the role to assume.
	// It takes precedence over internalBucket.
	roleARN string

	// internalBucket is the name of the internal bucket to get credentials for.
	internalBucket string
}

// createEvergreenCredentials creates a new evergreenCredentialProvider. It supports
// long operations or operations that might need to request new credentials during
// the operation (e.g. multipart bucket uploads).
func createEvergreenCredentials(comm client.Communicator, taskData client.TaskData, roleARN, internalBucket string) *evergreenCredentialProvider {
	return &evergreenCredentialProvider{
		comm:           comm,
		taskData:       taskData,
		roleARN:        roleARN,
		internalBucket: internalBucket,
	}
}

func (p *evergreenCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	var creds *apimodels.AWSCredentials
	var err error

	if p.roleARN != "" {
		creds, err = p.comm.AssumeRole(ctx, p.taskData, apimodels.AssumeRoleRequest{
			RoleARN: p.roleARN,
		})
	} else if p.internalBucket != "" {
		creds, err = p.comm.S3Credentials(ctx, p.taskData, p.internalBucket)
	} else {
		return aws.Credentials{}, errors.New("no role ARN or internal bucket provided")
	}

	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "getting S3 credentials")
	}
	if creds == nil {
		return aws.Credentials{}, errors.New("nil credentials returned")
	}

	expires, err := time.Parse(time.RFC3339, creds.Expiration)
	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "parsing expiration time")
	}

	return aws.Credentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Expires:         expires,
	}, nil
}
