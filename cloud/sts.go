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
	"github.com/evergreen-ci/utility/ttlcache"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// STSManager is an interface which handles STS operations.
// It's main purpose is to expose a friendly API for our own API server.
type STSManager interface {
	// AssumeRole gets the credentials for a role as the given task.
	AssumeRole(ctx context.Context, taskID string, opts AssumeRoleOptions) (AssumeRoleCredentials, error)
	// GetCallerIdentityARN gets the caller identity's ARN.
	GetCallerIdentityARN(ctx context.Context) (string, error)
}

// GetSTSManager returns either a real or mock STSManager.
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

// AssumeRoleOptions are the options for assuming a role.
// Some internal options are not present and are set by the manager
// (e.g. ExternalID).
type AssumeRoleOptions struct {
	// RoleARN is the Amazon Resource Name (ARN) of the role to assume.
	RoleARN string
	// Policy is an optional field that can be used to restrict the permissions.
	Policy *string
	// DurationSeconds is an optional field of the duration of the role session.
	// It defaults to 15 minutes.
	DurationSeconds *int32
	// CanCache signals whether to cache the credentials.
	CanCache bool
}

// AssumeRoleCredentials are the credentials to be returned from
// assuming a role.
type AssumeRoleCredentials struct {
	AccessKeyID     string
	SecretAccessKey string
	SessionToken    string
	Expiration      time.Time
}

// assumeRoleCache holds AWSCredentials for assumed roles.
var assumeRoleCache = ttlcache.WithOtel(ttlcache.NewInMemory[AssumeRoleCredentials](), "aws-assume-role")

// minAssumeRoleCacheLifetime is the minimum lifetime of an assumed role
// when retrieved from the cache. This can be used in situations where it's
// known that the credentials don't need to be used for a long time. Such as
// S3 operations that only need valid credentials at the beginning of the operations.
const minAssumeRoleCacheLifetime = 2 * time.Minute

// AssumeRole gets the credentials for a role as the given task. It handles
// the AWS API call and generating the ExternalID for the request.
func (s *stsManagerImpl) AssumeRole(ctx context.Context, taskID string, opts AssumeRoleOptions) (AssumeRoleCredentials, error) {
	if err := s.client.Create(ctx, evergreen.DefaultEC2Region); err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "creating AWS client")
	}
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "finding task")
	}
	if t == nil {
		return AssumeRoleCredentials{}, errors.New("task not found")
	}
	externalID := createExternalID(t)
	cacheID := externalID
	if opts.Policy != nil {
		cacheID += *opts.Policy
	}
	if opts.CanCache {
		if output, found := assumeRoleCache.Get(ctx, cacheID, minAssumeRoleCacheLifetime); found {
			return output, nil
		}
	}
	output, err := s.client.AssumeRole(ctx, &sts.AssumeRoleInput{
		RoleArn:         &opts.RoleARN,
		Policy:          opts.Policy,
		DurationSeconds: opts.DurationSeconds,
		ExternalId:      aws.String(externalID),
		RoleSessionName: aws.String(strconv.Itoa(int(time.Now().Unix()))),
	})
	if err != nil {
		return AssumeRoleCredentials{}, errors.Wrapf(err, "assuming role")
	}
	if err := validateAssumeRoleOutput(output); err != nil {
		return AssumeRoleCredentials{}, errors.Wrap(err, "validating assume role output")
	}
	creds := AssumeRoleCredentials{
		AccessKeyID:     *output.Credentials.AccessKeyId,
		SecretAccessKey: *output.Credentials.SecretAccessKey,
		SessionToken:    *output.Credentials.SessionToken,
		Expiration:      *output.Credentials.Expiration,
	}
	if opts.CanCache {
		assumeRoleCache.Put(ctx, cacheID, creds, creds.Expiration)
	}
	return creds, nil
}

// GetCallerIdentityARN gets the caller identity's ARN.
func (s *stsManagerImpl) GetCallerIdentityARN(ctx context.Context) (string, error) {
	if err := s.client.Create(ctx, evergreen.DefaultEC2Region); err != nil {
		return "", errors.Wrapf(err, "creating AWS client")
	}
	output, err := s.client.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return "", errors.Wrapf(err, "assuming role")
	}
	if output.Arn == nil {
		return "", errors.New("caller identity ARN is nil")
	}
	return *output.Arn, nil
}

func createExternalID(task *task.Task) string {
	// The external ID is used as a trust boundary for the AssumeRole call.
	// It is an unconfigurable computed value from the task of its project and
	// requester to avoid the confused deputy problem since Evergreen
	// assumes many roles on behalf of tasks.
	return fmt.Sprintf("%s-%s", task.Project, task.Requester)
}

func validateAssumeRoleOutput(assumeRole *sts.AssumeRoleOutput) error {
	if assumeRole == nil || assumeRole.Credentials == nil {
		return errors.New("assume role output is nil")
	}
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(assumeRole.Credentials.AccessKeyId == nil, "access key ID is nil")
	catcher.NewWhen(assumeRole.Credentials.SecretAccessKey == nil, "secret access key is nil")
	catcher.NewWhen(assumeRole.Credentials.SessionToken == nil, "session token is nil")
	catcher.NewWhen(assumeRole.Credentials.Expiration == nil, "expiration is nil")
	return catcher.Resolve()
}
