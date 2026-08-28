package command

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/pkg/errors"
)

// sourceCacheCredentialProvider is backed by the source cache credentials route.
// Unlike evergreenCredentialProvider it sends no role ARN and no policy; the app
// server picks the role and scopes the credentials to the task's own prefix.
type sourceCacheCredentialProvider struct {
	comm     client.Communicator
	taskData client.TaskData
}

// newCachedSourceCacheCredentials caches the credentials so a role is not assumed
// per S3 request.
func newCachedSourceCacheCredentials(comm client.Communicator, taskData client.TaskData) aws.CredentialsProvider {
	return aws.NewCredentialsCache(&sourceCacheCredentialProvider{comm: comm, taskData: taskData})
}

func (p *sourceCacheCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	creds, err := p.comm.SourceCacheCredentials(ctx, p.taskData)
	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "getting source cache credentials")
	}
	if creds == nil {
		return aws.Credentials{}, errors.New("nil source cache credentials returned")
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
		CanExpire:       true,
	}, nil
}
