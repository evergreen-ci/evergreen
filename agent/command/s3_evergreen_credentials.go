package command

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/pkg/errors"
)

const (
	expiryWindow = 5 * time.Second
)

// evergreenCredentialProvider is an AWS credential provider that
// retrieves credentials from Evergreen.
type evergreenCredentialProvider struct {
	comm     client.Communicator
	taskData client.TaskData

	// existingCredentials is the existing credentials to use.
	existingCredentials *aws.Credentials

	// roleARN is the ARN of the role to assume.
	roleARN string

	// updateExternalID is a function that gets called with
	// the external ID used in the AssumeRole request.
	updateExternalID func(string)
}

// createEvergreenCredentials creates a new evergreenCredentialProvider. It supports
// long operations or operations that might need to request new credentials during
// the operation (e.g. multipart bucket uploads).
func createEvergreenCredentials(comm client.Communicator, taskData client.TaskData, existingCredentials *aws.Credentials, roleARN string, updateExternalID func(string)) *evergreenCredentialProvider {
	if existingCredentials != nil && existingCredentials.CanExpire {
		// Typically, AWS allows an expiry window when creating the client. The expiry window prevents
		// credentials expiring timing issues. However, the expiry window wouldn't work with this credential
		// provider since we may be getting credentials that are really close to the expiration time.
		// The below is essentially a manual expiry window of 5 seconds. This should be enough to allow
		// an operation to start. AWS's SDK will handle when they expire and call Retrieve again.
		existingCredentials.Expires = existingCredentials.Expires.Add(-expiryWindow)
	}

	return &evergreenCredentialProvider{
		comm:                comm,
		taskData:            taskData,
		existingCredentials: existingCredentials,
		roleARN:             roleARN,
		updateExternalID:    updateExternalID,
	}
}

func (p *evergreenCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	if p.existingCredentials != nil && p.existingCredentials.HasKeys() && !p.existingCredentials.Expired() {
		return *p.existingCredentials, nil
	}

	if p.roleARN == "" {
		return aws.Credentials{}, errors.New("no role ARN provided")
	}

	creds, err := p.comm.AssumeRole(ctx, p.taskData, apimodels.AssumeRoleRequest{
		RoleARN: p.roleARN,
	})

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

	if p.updateExternalID != nil {
		p.updateExternalID(creds.ExternalID)
	}

	return aws.Credentials{
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Expires:         expires,
		CanExpire:       true,
	}, nil
}
