package command

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	s3GetAttribute = "evergreen.command.s3_get"
)

var (
	s3GetBucketAttribute               = fmt.Sprintf("%s.bucket", s3GetAttribute)
	s3GetTemporaryCredentialsAttribute = fmt.Sprintf("%s.temporary_credentials", s3GetAttribute)
	s3GetRemoteFileAttribute           = fmt.Sprintf("%s.remote_file", s3GetAttribute)
	s3GetExpandedRemoteFileAttribute   = fmt.Sprintf("%s.expanded_remote_file", s3GetAttribute)
)

// s3get is a command to fetch a resource from an S3 bucket and download it to
// the local machine.
type s3get struct {
	// AwsKey, AwsSecret, and AwsSessionToken are the user's credentials for
	// authenticating interactions with s3.
	AwsKey          string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret       string `mapstructure:"aws_secret" plugin:"expand"`
	AwsSessionToken string `mapstructure:"aws_session_token" plugin:"expand"`

	// RemoteFile is the file path of the file to get, within its bucket.
	RemoteFile string `mapstructure:"remote_file" plugin:"expand"`

	// Region is the S3 region where the bucket is located. It defaults to
	// "us-east-1".
	Region string `mapstructure:"region" plugin:"region"`

	// Bucket is the S3 bucket holding the desired file.
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// BuildVariants stores a list of build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants" plugin:"expand"`

	// Only one of these two should be specified. local_file indicates that the
	// s3 resource should be downloaded as-is to the specified file, and
	// extract_to indicates that the remote resource is a .tgz file to be
	// downloaded to the specified directory.
	LocalFile string `mapstructure:"local_file" plugin:"expand"`
	ExtractTo string `mapstructure:"extract_to" plugin:"expand"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// the path specified in remote_file does not exist. Defaults to false, which triggers errors
	// for missing files.
	Optional string `mapstructure:"optional" plugin:"expand"`

	// TemporaryRoleARN is not meant to be used in production. It is used for testing purposes
	// relating to the DEVPROD-5553 project.
	// This is an ARN that should be assumed to make the S3 request.
	// TODO (DEVPROD-13982): Upgrade this flag to RoleARN.
	TemporaryRoleARN string `mapstructure:"temporary_role_arn" plugin:"expand"`

	bucket          pail.Bucket
	internalBuckets []string

	skipMissing        bool
	originalRemoteFile string

	taskdata client.TaskData
	base
}

func s3GetFactory() Command   { return &s3get{} }
func (c *s3get) Name() string { return "s3.get" }
func (c *s3get) ParseParams(params map[string]interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           c,
	})
	if err != nil {
		return errors.Wrap(err, "initializing mapstructure decoder")
	}
	if err := decoder.Decode(params); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating params")
	}
	c.originalRemoteFile = c.RemoteFile

	return nil
}

func (c *s3get) validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(errors.Wrapf(validateS3BucketName(c.Bucket), "validating bucket name '%s'", c.Bucket))
	catcher.NewWhen(c.LocalFile != "" && c.ExtractTo != "", "cannot specify both local file path and directory to extract to")
	catcher.NewWhen(c.LocalFile == "" && c.ExtractTo == "", "must specify either local file path or directory to extract to")

	if c.TemporaryRoleARN != "" {
		// When using the role ARN, there should be no provided AWS credentials.
		catcher.NewWhen(c.AwsKey != "", "AWS key must be empty when using role ARN")
		catcher.NewWhen(c.AwsSecret != "", "AWS secret must be empty when using role ARN")
		catcher.NewWhen(c.AwsSessionToken != "", "AWS session token must be empty when using role ARN")
	}

	// If the bucket is not an internal bucket, the AWS credentials must be provided.
	// The internalBuckets field is only populated during runtime so commands do not
	// require credentials during validation.
	if len(c.internalBuckets) > 0 && !utility.StringSliceContains(c.internalBuckets, c.Bucket) {
		catcher.NewWhen(c.AwsKey == "", "AWS key must be provided")
		catcher.NewWhen(c.AwsSecret == "", "AWS secret must be provided")
	}

	return catcher.Resolve()
}

func (c *s3get) expandParams(conf *internal.TaskConfig) error {
	var err error
	if err = util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}
	if c.Optional != "" {
		c.skipMissing, err = strconv.ParseBool(c.Optional)
		if err != nil {
			return errors.Wrap(err, "parsing optional parameter as a boolean")
		}
	}
	if c.LocalFile != "" {
		c.LocalFile = GetWorkingDirectory(conf, c.LocalFile)
	}
	if c.ExtractTo != "" {
		c.ExtractTo = GetWorkingDirectory(conf, c.ExtractTo)
	}
	if c.Region == "" {
		c.Region = evergreen.DefaultEC2Region
	}
	return nil
}

func (c *s3get) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	c.internalBuckets = conf.InternalBuckets
	c.taskdata = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "expanding params")
	}
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating expanded params")
	}

	if !shouldRunForVariant(c.BuildVariants, conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 get of remote file '%s' for variant '%s'.", c.RemoteFile, conf.BuildVariant.Name)
		return nil
	}

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String(s3GetBucketAttribute, c.Bucket),
		attribute.Bool(s3GetTemporaryCredentialsAttribute, c.AwsSessionToken != ""),
		attribute.String(s3GetRemoteFileAttribute, c.originalRemoteFile),
		attribute.String(s3GetExpandedRemoteFileAttribute, c.RemoteFile),
	)

	if c.LocalFile != "" {
		if err := createEnclosingDirectoryIfNeeded(c.LocalFile); err != nil {
			return errors.Wrapf(err, "creating parent directories for local file '%s'", c.LocalFile)
		}
	}
	if c.ExtractTo != "" {
		if err := createEnclosingDirectoryIfNeeded(c.ExtractTo); err != nil {
			return errors.Wrapf(err, "creating parent directories for extraction directory '%s'", c.ExtractTo)
		}
	}

	// If a role is provided, assume the role to get temporary credentials.
	if c.TemporaryRoleARN != "" {
		creds, err := comm.AssumeRole(ctx, c.taskdata, apimodels.AssumeRoleRequest{
			RoleARN: c.TemporaryRoleARN,
		})
		if err != nil {
			return errors.Wrap(err, "getting credentials for provided role arn")
		}
		if creds == nil {
			return errors.New("nil credentials returned for provided role arn")
		}
		c.AwsKey = creds.AccessKeyID
		c.AwsSecret = creds.SecretAccessKey
		c.AwsSessionToken = creds.SessionToken
	}

	// create pail bucket
	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)

	if err := c.createPailBucket(ctx, httpClient); err != nil {
		return errors.Wrap(err, "creating S3 bucket")
	}

	err := c.getWithRetry(ctx, logger)
	if err != nil && c.skipMissing {
		logger.Task().Infof("Problem getting file but optional is true, exiting without error (%s).", err.Error())
		return nil
	}

	return errors.Wrap(err, "getting file")
}

func (c *s3get) createPailBucket(ctx context.Context, httpClient *http.Client) error {
	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(c.AwsKey, c.AwsSecret, c.AwsSessionToken),
		Region:      c.Region,
		Name:        c.Bucket,
	}
	var err error
	c.bucket, err = pail.NewS3BucketWithHTTPClient(ctx, httpClient, opts)
	if err != nil {
		return errors.Wrap(err, "creating S3 bucket")
	}
	return errors.Wrap(c.bucket.Check(ctx), "checking bucket")
}

func (c *s3get) getWithRetry(ctx context.Context, logger client.LoggerProducer) error {
	backoffCounter := getS3OpBackoff()
	timer := time.NewTimer(0)
	defer timer.Stop()

	for i := 1; i <= maxS3OpAttempts; i++ {
		logger.Task().Infof("Fetching remote file '%s' from S3 bucket '%s' (attempt %d of %d).",
			c.RemoteFile, c.Bucket, i, maxS3OpAttempts)

		select {
		case <-ctx.Done():
			return errors.Errorf("canceled while running command '%s'", c.Name())
		case <-timer.C:
			var err error
			if c.LocalFile != "" {
				err = c.getLocalFile(ctx)
			} else {
				err = c.getExtractTo(ctx)
			}
			if err == nil {
				return nil
			}

			logger.Task().Errorf("Problem getting remote file '%s' from S3 bucket, retrying: %s", c.RemoteFile, err)
			timer.Reset(backoffCounter.Duration())
		}
	}

	return errors.Errorf("command '%s' failed after %d attempts", c.Name(), maxS3OpAttempts)
}

func (c *s3get) getLocalFile(ctx context.Context) error {
	if utility.FileExists(c.LocalFile) {
		if err := os.RemoveAll(c.LocalFile); err != nil {
			return errors.Wrapf(err, "removing already-existing local file '%s'", c.LocalFile)
		}
	}

	return errors.Wrapf(c.bucket.Download(ctx, c.RemoteFile, c.LocalFile),
		"downloading remote file '%s' to local file '%s'", c.RemoteFile, c.LocalFile)
}

func (c *s3get) getExtractTo(ctx context.Context) error {
	reader, err := c.bucket.Reader(ctx, c.RemoteFile)
	if err != nil {
		return errors.Wrapf(err, "getting reader for remote file '%s'", c.RemoteFile)
	}

	return errors.Wrapf(extractTarball(ctx, reader, c.ExtractTo, []string{}),
		"extracting file '%s' from archive to destination '%s'", c.RemoteFile, c.ExtractTo)
}
