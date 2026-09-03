package command

import (
	"context"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/pkg/errors"
)

// sourceCacheCredentialProvider is backed by the source cache credentials route.
// Unlike evergreenCredentialProvider it sends no role ARN and no policy; the app
// server picks the role and scopes the credentials to the task's own prefix.
type sourceCacheCredentialProvider struct {
	comm     client.Communicator
	taskData client.TaskData
	request  apimodels.SourceCacheCredentialsRequest

	// initial is the response the task already fetched while building the cache,
	// so it is not requested again on the first S3 call.
	initial *aws.Credentials
}

// newCachedSourceCacheCredentials returns a provider seeded with the route
// response the task already fetched, so a new role is not assumed from scratch on
// the first S3 request. Refreshes hit the route with the same request, so the
// renewed grant matches the original one.
func newCachedSourceCacheCredentials(comm client.Communicator, taskData client.TaskData, request apimodels.SourceCacheCredentialsRequest, initial *apimodels.SourceCacheCredentialsResponse) (aws.CredentialsProvider, error) {
	p := &sourceCacheCredentialProvider{comm: comm, taskData: taskData, request: request}
	if initial == nil {
		return aws.NewCredentialsCache(p), nil
	}
	creds, err := sourceCacheAWSCredentials(initial)
	if err != nil {
		return nil, err
	}
	p.initial = &creds
	return aws.NewCredentialsCache(p), nil
}

func (p *sourceCacheCredentialProvider) Retrieve(ctx context.Context) (aws.Credentials, error) {
	if p.initial != nil && time.Until(p.initial.Expires) > 0 {
		return *p.initial, nil
	}
	resp, err := p.comm.SourceCacheCredentials(ctx, p.taskData, p.request)
	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "getting source cache credentials")
	}
	return sourceCacheAWSCredentials(resp)
}

func sourceCacheAWSCredentials(resp *apimodels.SourceCacheCredentialsResponse) (aws.Credentials, error) {
	if resp == nil {
		return aws.Credentials{}, errors.New("nil source cache credentials returned")
	}
	expires, err := time.Parse(time.RFC3339, resp.Expiration)
	if err != nil {
		return aws.Credentials{}, errors.Wrap(err, "parsing expiration time")
	}
	return aws.Credentials{
		AccessKeyID:     resp.AccessKeyID,
		SecretAccessKey: resp.SecretAccessKey,
		SessionToken:    resp.SessionToken,
		Expires:         expires,
		CanExpire:       true,
	}, nil
}
