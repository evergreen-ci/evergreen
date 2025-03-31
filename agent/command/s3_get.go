package command

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
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
	s3GetRoleARN                       = fmt.Sprintf("%s.role_arn", s3GetAttribute)
	s3GetInternalBucketAttribute       = fmt.Sprintf("%s.internal_bucket", s3GetAttribute)
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

	// remoteFile is the file path without any expansions applied.
	remoteFile string

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

	// TemporaryUseInternalBucket is not meant to be used in production. It is used for testing purposes
	// relating to the DEVPROD-5553 project.
	// This flag is used to determine if the s3_credentials route should be called before the command is executed.
	// TODO (DEVPROD-13982): Remove this flag and use the internal bucket list to determine if the s3_credentials
	// route should be called.
	TemporaryUseInternalBucket string `mapstructure:"temporary_use_internal_bucket" plugin:"expand"`

	skipMissing bool

	bucket          pail.FastGetS3Bucket
	internalBuckets []string

	temporaryUseInternalBucket bool

	base
}

func s3GetFactory() Command   { return &s3get{} }
func (c *s3get) Name() string { return "s3.get" }

// s3get implementation of ParseParams.
func (c *s3get) ParseParams(params map[string]any) error {
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

	// make sure the command params are valid
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating params")
	}

	return nil
}

// validate that all necessary params are set, and that only one of
// local_file and extract_to is specified.
func (c *s3get) validate() error {
	catcher := grip.NewSimpleCatcher()

	if c.TemporaryRoleARN != "" {
		// When using the role ARN, there should be no provided AWS credentials.
		catcher.NewWhen(c.AwsKey != "", "AWS key must be empty when using role ARN")
		catcher.NewWhen(c.AwsSecret != "", "AWS secret must be empty when using role ARN")
		catcher.NewWhen(c.AwsSessionToken != "", "AWS session token must be empty when using role ARN")
	} else {
		catcher.NewWhen(c.AwsKey == "", "AWS key cannot be blank")
		catcher.NewWhen(c.AwsSecret == "", "AWS secret cannot be blank")
	}

	catcher.NewWhen(c.RemoteFile == "", "remote file cannot be blank")
	catcher.Wrapf(validateS3BucketName(c.Bucket), "validating bucket name '%s'", c.Bucket)

	// There must be only one of local_file or extract_to specified.
	catcher.NewWhen(c.LocalFile != "" && c.ExtractTo != "", "cannot specify both local file path and directory to extract to")
	catcher.NewWhen(c.LocalFile == "" && c.ExtractTo == "", "must specify either local file path or directory to extract to")

	return catcher.Resolve()
}

// Apply the expansions from the relevant task config
// to all appropriate fields of the s3get.
func (c *s3get) expandParams(conf *internal.TaskConfig) error {
	c.remoteFile = c.RemoteFile

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

	if c.TemporaryUseInternalBucket != "" {
		c.temporaryUseInternalBucket, err = strconv.ParseBool(c.TemporaryUseInternalBucket)
		if err != nil {
			return errors.Wrap(err, "parsing temporary use internal bucket parameter as a boolean")
		}
	}

	if c.Region == "" {
		c.Region = evergreen.DefaultEC2Region
	}

	return nil
}

// Execute expands the parameters, and then fetches the
// resource from s3.
func (c *s3get) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	c.internalBuckets = conf.InternalBuckets

	// expand necessary params
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "expanding params")
	}

	// validate the params
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating expanded params")
	}

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String(s3GetBucketAttribute, c.Bucket),
		attribute.Bool(s3GetTemporaryCredentialsAttribute, c.AwsSessionToken != ""),
		attribute.String(s3GetRemoteFileAttribute, c.remoteFile),
		attribute.String(s3GetExpandedRemoteFileAttribute, c.RemoteFile),
		attribute.String(s3GetRoleARN, c.TemporaryRoleARN),
		attribute.Bool(s3GetInternalBucketAttribute, utility.StringSliceContains(conf.InternalBuckets, c.Bucket)),
	)

	taskData := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	if c.TemporaryRoleARN != "" {
		creds, err := comm.AssumeRole(ctx, taskData, apimodels.AssumeRoleRequest{
			RoleARN: c.TemporaryRoleARN,
		})
		if err != nil {
			return errors.Wrapf(err, "getting credentials for '%s' role arn", c.TemporaryRoleARN)
		}
		if creds == nil {
			return errors.Errorf("nil credentials returned for '%s' role arn", c.TemporaryRoleARN)
		}
		c.AwsKey = creds.AccessKeyID
		c.AwsSecret = creds.SecretAccessKey
		c.AwsSessionToken = creds.SessionToken
	}

	if c.temporaryUseInternalBucket {
		creds, err := comm.S3Credentials(ctx, taskData, c.Bucket)
		if err != nil {
			return errors.Wrap(err, "getting S3 credentials")
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
	err := c.createPailBucket(ctx, httpClient)
	if err != nil {
		return errors.Wrap(err, "creating S3 bucket")
	}

	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "checking bucket")
	}

	if !shouldRunForVariant(c.BuildVariants, conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 get of remote file '%s' for variant '%s'.",
			c.RemoteFile, conf.BuildVariant.Name)
		return nil
	}

	// if the local file or extract_to is a relative path, join it to the
	// working dir
	if c.LocalFile != "" {
		if !filepath.IsAbs(c.LocalFile) {
			c.LocalFile = GetWorkingDirectory(conf, c.LocalFile)
		}

		if err := createEnclosingDirectoryIfNeeded(c.LocalFile); err != nil {
			return errors.Wrapf(err, "creating parent directories for local file '%s'", c.LocalFile)
		}
	}

	if c.ExtractTo != "" {
		if !filepath.IsAbs(c.ExtractTo) {
			c.ExtractTo = GetWorkingDirectory(conf, c.ExtractTo)
		}

		if err := createEnclosingDirectoryIfNeeded(c.ExtractTo); err != nil {
			return errors.Wrapf(err, "creating parent directories for extraction directory '%s'", c.ExtractTo)
		}
	}

	errChan := make(chan error)
	go func() {
		err := errors.WithStack(c.getWithRetry(ctx, logger))
		select {
		case errChan <- err:
			return
		case <-ctx.Done():
			logger.Task().Infof("Context canceled waiting for s3 get: %s.", ctx.Err())
			return
		}
	}()

	select {
	case err := <-errChan:
		if err != nil && c.skipMissing {
			logger.Task().Infof("Problem getting file but optional is true, exiting without error (%s).", err.Error())
			return nil
		}
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Infof("Canceled while running command '%s': %s", c.Name(), ctx.Err())
		return nil
	}

}

// Wrapper around the Get() function to retry it
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
			err := errors.WithStack(c.get(ctx))
			if err == nil {
				return nil
			}

			logger.Task().Errorf("Problem getting remote file '%s' from S3 bucket, retrying: %s",
				c.RemoteFile, err)
			timer.Reset(backoffCounter.Duration())
		}
	}

	return errors.Errorf("command '%s' failed after %d attempts", c.Name(), maxS3OpAttempts)
}

// Fetch the specified resource from s3.
func (c *s3get) get(ctx context.Context) error {
	// either untar the remote, or just write to a file
	if c.LocalFile != "" {
		// remove the file, if it exists
		if utility.FileExists(c.LocalFile) {
			if err := os.RemoveAll(c.LocalFile); err != nil {
				return errors.Wrapf(err, "removing already-existing local file '%s'", c.LocalFile)
			}
		}

		if err := os.MkdirAll(filepath.Dir(c.LocalFile), 0700); err != nil {
			return errors.Wrapf(err, "creating enclosing directory for file '%s'", c.LocalFile)
		}

		f, err := os.Create(c.LocalFile)
		if err != nil {
			return errors.Wrapf(err, "creating file '%s'", c.LocalFile)
		}

		defer f.Close()

		if err := c.bucket.GetToWriter(ctx, c.RemoteFile, f); err != nil {
			return errors.Wrapf(err, "downloading remote file '%s' to local file '%s'", c.RemoteFile, c.LocalFile)
		}

		return nil
	}

	f, err := os.CreateTemp(filepath.Dir(c.ExtractTo), "temp-get-*.tgz")
	if err != nil {
		return errors.Wrapf(err, "creating temporary file")
	}

	defer f.Close()

	catcher := grip.NewBasicCatcher()

	catcher.Wrapf(c.fetchAndExtractTarball(ctx, f), "fetching and extracting targz")
	catcher.Wrapf(os.Remove(f.Name()), "removing temporary file")

	return catcher.Resolve()
}

func (c *s3get) fetchAndExtractTarball(ctx context.Context, f *os.File) error {
	if err := c.bucket.GetToWriter(ctx, c.RemoteFile, f); err != nil {
		return errors.Wrapf(err, "downloading remote file '%s' to local file '%s'", c.RemoteFile, f.Name())
	}

	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return errors.Wrapf(err, "seeking to start of tgz file")
	}

	if err := extractTarball(ctx, f, c.ExtractTo, []string{}); err != nil {
		return errors.Wrapf(err, "extracting file '%s' from archive to destination '%s'", c.RemoteFile, c.ExtractTo)
	}

	return nil
}

func (c *s3get) createPailBucket(ctx context.Context, httpClient *http.Client) error {
	opts := pail.S3Options{
		Credentials: pail.CreateAWSStaticCredentials(c.AwsKey, c.AwsSecret, c.AwsSessionToken),
		Region:      c.Region,
		Name:        c.Bucket,
	}
	bucket, err := pail.NewFastGetS3BucketWithHTTPClient(ctx, httpClient, opts)
	c.bucket = bucket
	return err
}
