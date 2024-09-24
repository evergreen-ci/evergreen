package command

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type ec2AssumeRole struct {
	// The Amazon Resource Name (ARN) of the role to assume.
	// Required.
	RoleARN string `mapstructure:"role_arn" plugin:"expand"`

	// An IAM policy in JSON format that you want to use as an inline session policy.
	Policy string `mapstructure:"policy" plugin:"expand"`

	// The duration, in seconds, of the role session.
	// Defaults to 900s (15 minutes).
	DurationSeconds int32 `mapstructure:"duration_seconds"`

	// TemporaryFeatureFlag is a flag to flip between the new and old implementation.
	// TODO (DEVPROD-9947): Remove this.
	TemporaryFeatureFlag bool `mapstructure:"temporary_feature_flag"`

	base
}

func ec2AssumeRoleFactory() Command   { return &ec2AssumeRole{} }
func (r *ec2AssumeRole) Name() string { return "ec2.assume_role" }

func (r *ec2AssumeRole) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, r); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return r.validate()
}

func (r *ec2AssumeRole) validate() error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(r.RoleARN == "", "must specify role ARN")
	// 0 will default duration time to 15 minutes.
	catcher.NewWhen(r.DurationSeconds < 0, "cannot specify a non-positive duration")
	return catcher.Resolve()
}

func (r *ec2AssumeRole) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(r, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}
	// Re-validate the command here, in case an expansion is not defined.
	if err := r.validate(); err != nil {
		return errors.WithStack(err)
	}

	// TODO (DEVPROD-9947): Remove feature flag check and just use the new implementation.
	if r.TemporaryFeatureFlag {
		return r.execute(ctx, comm, logger, conf)
	}

	return r.legacyExecute(ctx, comm, logger, conf)
}

func (r *ec2AssumeRole) execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	request := apimodels.AssumeRoleRequest{
		RoleARN: r.RoleARN,
	}
	if r.DurationSeconds > 0 {
		request.DurationSeconds = utility.ToInt32Ptr(r.DurationSeconds)
	}
	if r.Policy != "" {
		request.Policy = utility.ToStringPtr(r.Policy)
	}
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	creds, err := comm.AssumeRole(ctx, td, request)
	if err != nil {
		return errors.Wrap(err, "assuming role")
	}
	if creds == nil {
		return errors.New("nil credentials returned")
	}
	conf.NewExpansions.PutAndRedact(globals.AWSAccessKeyId, creds.AccessKeyID)
	conf.NewExpansions.PutAndRedact(globals.AWSSecretAccessKey, creds.SecretAccessKey)
	conf.NewExpansions.PutAndRedact(globals.AWSSessionToken, creds.SessionToken)
	conf.NewExpansions.PutAndRedact(globals.AWSRoleExpiration, creds.Expiration)

	return nil
}

func (r *ec2AssumeRole) legacyExecute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	// TODO (DEVPROD-9947): Remove this.
	if len(conf.EC2Keys) == 0 {
		return errors.New("no EC2 keys in config")
	}

	key := conf.EC2Keys[0].Key
	secret := conf.EC2Keys[0].Secret

	if key == "" || secret == "" {
		return errors.New("AWS key and secret must not be empty")
	}

	assumeRoleCreds := credentials.NewStaticCredentialsProvider(key, secret, "")
	assumeRoleClient := sts.New(sts.Options{
		Region:      evergreen.DefaultEC2Region,
		Credentials: assumeRoleCreds,
	})
	stsCreds := stscreds.NewAssumeRoleProvider(assumeRoleClient, r.RoleARN, func(opts *stscreds.AssumeRoleOptions) {
		opts.RoleSessionName = strconv.Itoa(int(time.Now().Unix()))
		// External ID is a combination of project ID and requester to avoid the
		// confused deputy problem. Mainline commits might have higher trust
		// than patches.
		opts.ExternalID = utility.ToStringPtr(fmt.Sprintf("%s-%s", conf.ProjectRef.Id, conf.Task.Requester))
		if r.Policy != "" {
			opts.Policy = utility.ToStringPtr(r.Policy)
		}
		if r.DurationSeconds != 0 {
			opts.Duration = time.Duration(r.DurationSeconds) * time.Second
		}
	})

	creds, err := stsCreds.Retrieve(ctx)
	if err != nil {
		return errors.Wrap(err, "retrieving sts credentials")
	}

	conf.NewExpansions.Put(globals.AWSAccessKeyId, creds.AccessKeyID)
	conf.NewExpansions.Put(globals.AWSSecretAccessKey, creds.SecretAccessKey)
	conf.NewExpansions.Put(globals.AWSSessionToken, creds.SessionToken)
	conf.NewExpansions.Put(globals.AWSRoleExpiration, creds.Expires.String())
	return nil
}
