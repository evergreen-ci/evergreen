package command

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// s3Operation isn't a command itself, but it contains the common parameters
// and logic for S3 commands.
type s3Operation struct {
	awsCredentials `plugin:"expand"`
	bucketOptions  `plugin:"expand"`

	// BuildVariants stores a list of build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// the path specified in remote_file does not exist. Defaults to false, which triggers errors
	// for missing files.
	Optional string `mapstructure:"optional" plugin:"expand"`
	// optional is the parsed boolean value of Optional.
	optional bool

	// TemporaryUseInternalBucket is not meant to be used in production. It is used for testing purposes
	// relating to the DEVPROD-5553 project.
	// This flag is used to determine if the s3_credentials route should be called before the command is executed.
	// TODO (DEVPROD-13982): Remove this flag and use the internal bucket list to determine if the s3_credentials
	// route should be called.
	TemporaryUseInternalBucket string `mapstructure:"temporary_use_internal_bucket" plugin:"expand"`
	// temporaryUseInternalBucket is the parsed boolean value of TemporaryUseInternalBucket.
	temporaryUseInternalBucket bool

	internalBuckets []string

	taskData client.TaskData
}

func (s *s3Operation) validate() []error {
	catcher := grip.NewBasicCatcher()

	catcher.Extend(s.awsCredentials.validate())
	catcher.Extend(s.bucketOptions.validate())

	return catcher.Errors()
}

func (s *s3Operation) expandParams(conf *internal.TaskConfig) error {
	if err := expandBool(s.Optional, &s.optional); err != nil {
		return errors.Wrap(err, "expanding optional")
	}

	if err := expandBool(s.TemporaryUseInternalBucket, &s.temporaryUseInternalBucket); err != nil {
		return errors.Wrap(err, "expanding temporary use internal bucket")
	}

	s.taskData = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	s.internalBuckets = conf.InternalBuckets
	s.bucketOptions.expandParams()

	if s.AWSSessionToken != "" && s.RoleARN == "" {
		// If no role was provided but a session token is being used (which means an AssumeRole credentials is being
		// used), check if the session token matches any saved from ec2.assume_role commands in the task config.
		// If it does, associate this command with the corresponding role ARN.
		if roleARN, ok := conf.AssumeRoleRoles[s.AWSSessionToken]; ok {
			s.assumeRoleARN = roleARN
		}
	}

	return nil
}

// createPailBucket completes the given options and creates the pail.Bucket using the
// given bucketFunc.
func (s *s3Operation) createPailBucket(opts pail.S3Options, comm client.Communicator, bucketFunc func(pail.S3Options) (pail.Bucket, error)) error {
	// No-op if the bucket is already created.
	if s.bucket != nil {
		return nil
	}

	opts.Region = s.Region
	opts.Name = s.Bucket

	if s.getRoleARN() != "" || s.temporaryUseInternalBucket {
		opts.Credentials = createEvergreenCredentials(comm, s.taskData, s.getRoleARN(), s.Bucket)
	} else if s.AWSKey != "" {
		opts.Credentials = pail.CreateAWSStaticCredentials(s.AWSKey, s.AWSSecret, s.AWSSessionToken)
	}

	var err error
	s.bucket, err = bucketFunc(opts)
	return err
}

// getRoleARN gets the role arn that's associated with this command,
// either from the passed in parameter or a previous ec2.assume_role command.
func (s *s3Operation) getRoleARN() string {
	if s.RoleARN != "" {
		return s.RoleARN
	}
	return s.assumeRoleARN
}

func (s *s3Operation) shouldRunForVariant(bv string) bool {
	return len(s.BuildVariants) == 0 || utility.StringSliceContains(s.BuildVariants, bv)
}

type awsCredentials struct {
	// AWSKey, AwsSecret, and AwsSessionToken are the user's static credentials for
	// authenticating interactions with S3.
	AWSKey          string `mapstructure:"aws_key" plugin:"expand"`
	AWSSecret       string `mapstructure:"aws_secret" plugin:"expand"`
	AWSSessionToken string `mapstructure:"aws_session_token" plugin:"expand"`

	// RoleARN is an ARN that should be assumed to make the S3 request.
	RoleARN string `mapstructure:"role_arn" plugin:"expand"`

	//assumeRoleARN is the ARN assoaciated with this command if the user provided
	// credentials came from an ec2.assume_role command.
	assumeRoleARN string
}

func (a *awsCredentials) validate() []error {
	catcher := grip.NewBasicCatcher()

	if a.RoleARN != "" {
		// When using the role ARN, there should be no provided AWS credentials.
		catcher.NewWhen(a.AWSKey != "", "AWS key must be empty when using role ARN")
		catcher.NewWhen(a.AWSSecret != "", "AWS secret must be empty when using role ARN")
		catcher.NewWhen(a.AWSSessionToken != "", "AWS session token must be empty when using role ARN")
	} else {
		catcher.NewWhen(a.AWSKey == "", "AWS key cannot be blank")
		catcher.NewWhen(a.AWSSecret == "", "AWS secret cannot be blank")
	}

	return catcher.Errors()
}

type bucketOptions struct {
	// Region is the S3 region where the bucket is located. It defaults to
	// "us-east-1".
	Region string `mapstructure:"region" plugin:"region"`

	// Bucket is the s3 bucket to use when storing the desired file.
	Bucket string `mapstructure:"bucket" plugin:"expand"`
	// bucket is the parsed pail bucket.
	bucket pail.Bucket

	// RemoteFile is the filepath to store the file to,
	// within an S3 bucket. Is a prefix when multiple files are uploaded via LocalFilesIncludeFilter.
	RemoteFile string `mapstructure:"remote_file" plugin:"expand"`
	// remoteFile is the file path without any expansions applied.
	remoteFile string
}

func (b *bucketOptions) validate() []error {
	catcher := grip.NewBasicCatcher()

	catcher.Wrapf(validateS3BucketName(b.Bucket), "invalid bucket name '%s'", b.Bucket)
	catcher.NewWhen(b.RemoteFile == "", "remote file cannot be blank")

	return catcher.Errors()
}

func (b *bucketOptions) expandParams() {
	if b.Region == "" {
		b.Region = evergreen.DefaultEC2Region
	}
}
