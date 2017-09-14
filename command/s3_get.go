package command

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// A plugin command to fetch a resource from an s3 bucket and download it to
// the local machine.
type s3get struct {
	// AwsKey and AwsSecret are the user's credentials for
	// authenticating interactions with s3.
	AwsKey    string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand"`

	// RemoteFile is the filepath of the file to get, within its bucket
	RemoteFile string `mapstructure:"remote_file" plugin:"expand"`

	// Bucket is the s3 bucket holding the desired file
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// BuildVariants stores a list of MCI build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants" plugin:"expand"`

	// Only one of these two should be specified. local_file indicates that the
	// s3 resource should be downloaded as-is to the specified file, and
	// extract_to indicates that the remote resource is a .tgz file to be
	// downloaded to the specified directory.
	LocalFile string `mapstructure:"local_file" plugin:"expand"`
	ExtractTo string `mapstructure:"extract_to" plugin:"expand"`

	base
}

func s3GetFactory() Command   { return &s3get{} }
func (c *s3get) Name() string { return "s3.get" }

// s3get-specific implementation of ParseParams.
func (c *s3get) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding %v params", c.Name())
	}

	// make sure the command params are valid
	if err := c.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating %v params", c.Name())
	}

	return nil
}

// Validate that all necessary params are set, and that only one of
// local_file and extract_to is specified.
func (c *s3get) validateParams() error {
	if c.AwsKey == "" {
		return errors.New("aws_key cannot be blank")
	}
	if c.AwsSecret == "" {
		return errors.New("aws_secret cannot be blank")
	}
	if c.RemoteFile == "" {
		return errors.New("remote_file cannot be blank")
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(c.Bucket); err != nil {
		return errors.Wrapf(err, "%v is an invalid bucket name", c.Bucket)
	}

	// make sure local file and extract-to dir aren't both specified
	if c.LocalFile != "" && c.ExtractTo != "" {
		return errors.New("cannot specify both local_file and extract_to directory")
	}

	// make sure one is specified
	if c.LocalFile == "" && c.ExtractTo == "" {
		return errors.New("must specify either local_file or extract_to")
	}
	return nil
}

func (c *s3get) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(c.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return util.SliceContains(c.BuildVariants, buildVariantName)
}

// Apply the expansions from the relevant task config to all appropriate
// fields of the s3get.
func (c *s3get) expandParams(conf *model.TaskConfig) error {
	return util.ExpandValues(c, conf.Expansions)
}

// Implementation of Execute.  Expands the parameters, and then fetches the
// resource from s3.
func (c *s3get) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	// expand necessary params
	if err := c.expandParams(conf); err != nil {
		return err
	}

	// validate the params
	if err := c.validateParams(); err != nil {
		return errors.Wrap(err, "expanded params are not valid")
	}

	if !c.shouldRunForVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 get of remote file %v for variant %v",
			c.RemoteFile, conf.BuildVariant.Name)
		return nil
	}

	// if the local file or extract_to is a relative path, join it to the
	// working dir
	if c.LocalFile != "" {
		if !filepath.IsAbs(c.LocalFile) {
			c.LocalFile = filepath.Join(conf.WorkDir, c.LocalFile)
		}

		if err := createEnclosingDirectoryIfNeeded(c.LocalFile); err != nil {
			return errors.WithStack(err)
		}
	}

	if c.ExtractTo != "" {
		if !filepath.IsAbs(c.ExtractTo) {
			c.ExtractTo = filepath.Join(conf.WorkDir, c.ExtractTo)
		}

		if err := createEnclosingDirectoryIfNeeded(c.ExtractTo); err != nil {
			return errors.WithStack(err)
		}
	}

	errChan := make(chan error)
	go func() {
		errChan <- errors.WithStack(c.getWithRetry(ctx, logger))
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of S3 Get Command")
		return nil
	}

}

// Wrapper around the Get() function to retry it
func (c *s3get) getWithRetry(ctx context.Context, logger client.LoggerProducer) error {
	backoffCounter := getS3OpBackoff()
	timer := time.NewTimer(0)
	defer timer.Stop()

	for i := 1; i <= maxS3OpAttempts; i++ {
		logger.Task().Infof("fetching %s from s3 bucket %s (attempt %d of %d)",
			c.RemoteFile, c.Bucket, i, maxS3OpAttempts)

		select {
		case <-ctx.Done():
			return errors.New("s3 get operation aborted")
		case <-timer.C:
			err := errors.WithStack(c.get(ctx))
			if err == nil {
				return nil
			}

			logger.Execution().Errorf("problem getting %s from s3 bucket, retrying. [%v]",
				c.RemoteFile, err)
			timer.Reset(backoffCounter.Duration())
		}
	}

	return errors.Errorf("S3 get failed after %d attempts", maxS3OpAttempts)
}

// Fetch the specified resource from s3.
func (c *s3get) get(ctx context.Context) error {
	// get the appropriate session and bucket
	auth := &aws.Auth{
		AccessKey: c.AwsKey,
		SecretKey: c.AwsSecret,
	}

	session := thirdparty.NewS3Session(auth, aws.USEast)
	bucket := session.Bucket(c.Bucket)

	// get a reader for the bucket
	reader, err := bucket.GetReader(c.RemoteFile)
	if err != nil {
		return errors.Wrapf(err, "error getting bucket reader for file %v", c.RemoteFile)
	}
	defer reader.Close()

	// either untar the remote, or just write to a file
	if c.LocalFile != "" {
		var exists bool
		// remove the file, if it exists
		exists, err = util.FileExists(c.LocalFile)
		if err != nil {
			return errors.Wrapf(err, "error checking existence of local file %v",
				c.LocalFile)
		}
		if exists {
			if err := os.RemoveAll(c.LocalFile); err != nil {
				return errors.Wrapf(err, "error clearing local file %v", c.LocalFile)
			}
		}

		// open the local file
		file, err := os.Create(c.LocalFile)
		if err != nil {
			return errors.Wrapf(err, "error opening local file %v", c.LocalFile)
		}
		defer file.Close()

		_, err = io.Copy(file, reader)
		return errors.WithStack(err)
	}

	// wrap the reader in a gzip reader and a tar reader
	gzipReader, err := gzip.NewReader(reader)
	if err != nil {
		return errors.Wrapf(err, "error creating gzip reader for %v", c.RemoteFile)
	}

	tarReader := tar.NewReader(gzipReader)
	err = util.Extract(ctx, tarReader, c.ExtractTo)
	if err != nil {
		return errors.Wrapf(err, "error extracting %v to %v", c.RemoteFile, c.ExtractTo)
	}

	return nil
}
