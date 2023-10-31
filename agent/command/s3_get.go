package command

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
)

// s3get is a command to fetch a resource from an S3 bucket and download it to
// the local machine.
type s3get struct {
	// AwsKey and AwsSecret are the user's credentials for
	// authenticating interactions with s3.
	AwsKey    string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand"`

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

	bucket pail.Bucket

	base
}

func s3GetFactory() Command   { return &s3get{} }
func (c *s3get) Name() string { return "s3.get" }

// s3get-specific implementation of ParseParams.
func (c *s3get) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	// make sure the command params are valid
	if err := c.validateParams(); err != nil {
		return errors.Wrap(err, "validating params")
	}

	return nil
}

// Validate that all necessary params are set, and that only one of
// local_file and extract_to is specified.
func (c *s3get) validateParams() error {
	if c.AwsKey == "" {
		return errors.New("AWS key cannot be blank")
	}
	if c.AwsSecret == "" {
		return errors.New("AWS secret cannot be blank")
	}
	if c.RemoteFile == "" {
		return errors.New("remote file cannot be blank")
	}

	if c.Region == "" {
		c.Region = endpoints.UsEast1RegionID
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(c.Bucket); err != nil {
		return errors.Wrapf(err, "validating bucket name '%s'", c.Bucket)
	}

	// make sure local file and extract-to dir aren't both specified
	if c.LocalFile != "" && c.ExtractTo != "" {
		return errors.New("cannot specify both local file path and directory to extract to")
	}

	// make sure one is specified
	if c.LocalFile == "" && c.ExtractTo == "" {
		return errors.New("must specify either local file path or directory to extract to")
	}

	return nil
}

func (c *s3get) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(c.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return utility.StringSliceContains(c.BuildVariants, buildVariantName)
}

// Apply the expansions from the relevant task config
// to all appropriate fields of the s3get.
func (c *s3get) expandParams(conf *internal.TaskConfig) error {
	return util.ExpandValues(c, &conf.Expansions)
}

// Implementation of Execute.  Expands the parameters, and then fetches the
// resource from s3.
func (c *s3get) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	// expand necessary params
	if err := c.expandParams(conf); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// validate the params
	if err := c.validateParams(); err != nil {
		return errors.Wrap(err, "validating expanded params")
	}

	// create pail bucket
	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)
	err := c.createPailBucket(httpClient)
	if err != nil {
		return errors.Wrap(err, "creating S3 bucket")
	}

	if err := c.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "checking bucket")
	}

	if !c.shouldRunForVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 get of remote file '%s' for variant '%s'.",
			c.RemoteFile, conf.BuildVariant.Name)
		return nil
	}

	// if the local file or extract_to is a relative path, join it to the
	// working dir
	if c.LocalFile != "" {
		if !filepath.IsAbs(c.LocalFile) {
			c.LocalFile = getWorkingDirectory(conf, c.LocalFile)
		}

		if err := createEnclosingDirectoryIfNeeded(c.LocalFile); err != nil {
			return errors.Wrapf(err, "creating parent directories for local file '%s'", c.LocalFile)
		}
	}

	if c.ExtractTo != "" {
		if !filepath.IsAbs(c.ExtractTo) {
			c.ExtractTo = getWorkingDirectory(conf, c.ExtractTo)
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

			logger.Execution().Errorf("Problem getting remote file '%s' from S3 bucket, retrying: %s",
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

		// download to local file
		return errors.Wrapf(c.bucket.Download(ctx, c.RemoteFile, c.LocalFile),
			"downloading remote file '%s' to local file '%s'", c.RemoteFile, c.LocalFile)

	}

	reader, err := c.bucket.Reader(ctx, c.RemoteFile)
	if err != nil {
		return errors.Wrapf(err, "getting reader for remote file '%s'", c.RemoteFile)
	}
	if err := agentutil.ExtractTarball(ctx, reader, c.ExtractTo, []string{}); err != nil {
		return errors.Wrapf(err, "extracting file '%s' from archive to destination '%s'", c.RemoteFile, c.ExtractTo)
	}

	return nil
}

func (c *s3get) createPailBucket(httpClient *http.Client) error {
	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(c.AwsKey, c.AwsSecret, ""),
		Region:      c.Region,
		Name:        c.Bucket,
	}
	bucket, err := pail.NewS3BucketWithHTTPClient(httpClient, opts)
	c.bucket = bucket
	return err
}
