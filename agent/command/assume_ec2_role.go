package command

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/credentials/stscreds"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	AwsAccessKeyId     = "AWS_ACCESS_KEY_ID"
	AwsSecretAccessKey = "AWS_SECRET_ACCESS_KEY"
	AwsSessionToken    = "AWS_SESSION_TOKEN"
	AwsRoleExpiration  = "AWS_ROLE_EXPIRATION"
)

type ec2AssumeRole struct {
	// The Amazon Resource Name (ARN) of the role to assume.
	// Required.
	RoleARN string `mapstructure:"role_arn" plugin:"expand"`

	// A unique identifier that might be required when you assume a role in another account.
	ExternalId string `mapstructure:"external_id" plugin:"expand"`

	// An IAM policy in JSON format that you want to use as an inline session policy.
	Policy string `mapstructure:"policy" plugin:"expand"`

	// The duration, in seconds, of the role session.
	// Defaults to 900s (15 minutes).
	DurationSeconds int `mapstructure:"duration_seconds"`

	base
}

func ec2AssumeRoleFactory() Command   { return &ec2AssumeRole{} }
func (r *ec2AssumeRole) Name() string { return "ec2.assume_role" }

func (r *ec2AssumeRole) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, r); err != nil {
		return errors.Wrapf(err, "error parsing '%s' params", r.Name())
	}

	return r.validate()
}

func (r *ec2AssumeRole) validate() error {
	catcher := grip.NewSimpleCatcher()

	if r.RoleARN == "" {
		catcher.New("must specify role ARN")
	}

	if r.DurationSeconds < 0 {
		catcher.New("cannot specify a non-positive duration")
	}

	return catcher.Resolve()
}

func (r *ec2AssumeRole) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(r, conf.Expansions); err != nil {
		return errors.WithStack(err)
	}
	// Re-validate the command here, in case an expansion is not defined.
	if err := r.validate(); err != nil {
		return errors.WithStack(err)
	}

	if len(conf.EC2Keys) == 0 {
		return errors.New("no EC2 keys in config")
	}

	key := conf.EC2Keys[0].Key
	secret := conf.EC2Keys[0].Secret

	// Error if key or secret are blank
	if key == "" || secret == "" {
		return errors.New("AWS ID and Secret must not be blank")
	}

	defaultCreds := credentials.NewStaticCredentialsFromCreds(credentials.Value{
		AccessKeyID:     key,
		SecretAccessKey: secret,
	})

	session1 := session.Must(session.NewSession(&aws.Config{
		Credentials: defaultCreds,
	}))

	creds := stscreds.NewCredentials(session1, r.RoleARN, func(arp *stscreds.AssumeRoleProvider) {
		arp.RoleSessionName = fmt.Sprintf("evergreen_%s_%d", conf.Task.DisplayName, conf.Task.Execution)
		if r.ExternalId != "" {
			arp.ExternalID = utility.ToStringPtr(r.ExternalId)
		}
		if r.Policy != "" {
			arp.Policy = utility.ToStringPtr(r.Policy)
		}
		if r.DurationSeconds != 0 {
			arp.Duration = time.Duration(r.DurationSeconds) * time.Second
		}
	})

	credValues, err := creds.Get()
	if err != nil {
		return errors.WithStack(err)
	}

	expTime, err := creds.ExpiresAt()
	if err != nil {
		return errors.WithStack(err)
	}

	conf.Expansions.Put(AwsAccessKeyId, credValues.AccessKeyID)
	conf.Expansions.Put(AwsSecretAccessKey, credValues.SecretAccessKey)
	conf.Expansions.Put(AwsSessionToken, credValues.SessionToken)
	conf.Expansions.Put(AwsRoleExpiration, expTime.String())
	return nil
}
