package cloud

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

type STSManager interface {
	// AssumeRole gets the credentials for a role as the given task.
	AssumeRole(ctx context.Context, taskID string, opts AssumeRoleOptions) (AssumeRoleCredentials, error)
}

func GetSTSManager(mock bool) STSManager {
	var client AWSClient = &awsClientImpl{}
	if mock {
		client = &awsClientMock{}
	}

	return &stsManagerImpl{
		client: client,
	}
}

type stsManagerImpl struct {
	client AWSClient
}

type AssumeRoleOptions struct {
	RoleARN         string
	Policy          string
	DurationSeconds *int32
}

type AssumeRoleCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

func (s *stsManagerImpl) AssumeRole(ctx context.Context, taskID string, opts AssumeRoleOptions) (AssumeRoleCredentials, error) {
	if err := s.client.Create(ctx, evergreen.DefaultEC2Region); err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "creating AWS client")
	}
	t, err := task.FindOneId(taskID)
	if err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "finding task")
	}
	if t == nil {
		return AssumeRoleCredentials{}, errors.New("task not found")
	}
	output, err := s.client.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         &opts.RoleARN,
		Policy:          &opts.Policy,
		DurationSeconds: opts.DurationSeconds,
		ExternalId:      aws.String(fmt.Sprintf("%s-%s", t.Project, t.Requester)),
		RoleSessionName: aws.String(strconv.Itoa(int(time.Now().Unix()))),
	})
	if err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "assuming role")
	}
	return AssumeRoleCredentials{
		AccessKeyID:     *output.Credentials.AccessKeyId,
		SecretAccessKey: *output.Credentials.SecretAccessKey,
		SessionToken:    *output.Credentials.SessionToken,
		Expiration:      *output.Credentials.Expiration,
	}, nil
}
