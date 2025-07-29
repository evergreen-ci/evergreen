package command

import (
	"context"
	"time"

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

	base
}

func ec2AssumeRoleFactory() Command   { return &ec2AssumeRole{} }
func (r *ec2AssumeRole) Name() string { return "ec2.assume_role" }

func (r *ec2AssumeRole) ParseParams(params map[string]any) error {
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
	conf.NewExpansions.Put(globals.AWSRoleExpiration, creds.Expiration)

	// Store the AssumeRoleInformation in the task config, so s3 operations can identify the corresponding
	// role ARN that the credentials are associated with.
	expiration, err := time.Parse(time.RFC3339, creds.Expiration)
	if err == nil {
		conf.AssumeRoleInformation[creds.SessionToken] = internal.AssumeRoleInformation{
			RoleARN:    r.RoleARN,
			Expiration: expiration,
		}
	} else {
		logger.Task().Warningf("Error parsing expiration time '%s' to determine if credentials can be cached: '%s'. Future operations will re-call AssumeRole.", creds.Expiration, err.Error())
	}

	return nil
}
