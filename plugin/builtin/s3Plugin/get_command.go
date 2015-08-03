package s3Plugin

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/archive"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/mitchellh/mapstructure"
	"io"
	"os"
	"path/filepath"
	"time"
)

var (
	MaxS3GetAttempts = 10
	S3GetSleep       = 5 * time.Second
)

// A plugin command to fetch a resource from an s3 bucket and download it to
// the local machine.
type S3GetCommand struct {
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
	BuildVariants []string `mapstructure:"build_variants"`

	// Only one of these two should be specified. local_file indicates that the
	// s3 resource should be downloaded as-is to the specified file, and
	// extract_to indicates that the remote resource is a .tgz file to be
	// downloaded to the specified directory.
	LocalFile string `mapstructure:"local_file" plugin:"expand"`
	ExtractTo string `mapstructure:"extract_to"`
}

func (self *S3GetCommand) Name() string {
	return S3GetCmd
}

func (self *S3GetCommand) Plugin() string {
	return S3PluginName
}

// S3GetCommand-specific implementation of ParseParams.
func (self *S3GetCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error decoding %v params: %v", self.Name(), err)
	}

	// make sure the command params are valid
	if err := self.validateParams(); err != nil {
		return fmt.Errorf("error validating %v params: %v", self.Name(), err)
	}

	return nil
}

// Validate that all necessary params are set, and that only one of
// local_file and extract_to is specified.
func (self *S3GetCommand) validateParams() error {
	if self.AwsKey == "" {
		return fmt.Errorf("aws_key cannot be blank")
	}
	if self.AwsSecret == "" {
		return fmt.Errorf("aws_secret cannot be blank")
	}
	if self.RemoteFile == "" {
		return fmt.Errorf("remote_file cannot be blank")
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(self.Bucket); err != nil {
		return fmt.Errorf("%v is an invalid bucket name: %v", self.Bucket, err)
	}

	// make sure local file and extract-to dir aren't both specified
	if self.LocalFile != "" && self.ExtractTo != "" {
		return fmt.Errorf("cannot specify both local_file and extract_to" +
			" directory")
	}

	// make sure one is specified
	if self.LocalFile == "" && self.ExtractTo == "" {
		return fmt.Errorf("must specify either local_file or extract_to")
	}
	return nil
}

func (self *S3GetCommand) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(self.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return util.SliceContains(self.BuildVariants, buildVariantName)
}

// Apply the expansions from the relevant task config to all appropriate
// fields of the S3GetCommand.
func (self *S3GetCommand) expandParams(conf *model.TaskConfig) error {
	return plugin.ExpandValues(self, conf.Expansions)
}

// Implementation of Execute.  Expands the parameters, and then fetches the
// resource from s3.
func (self *S3GetCommand) Execute(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator, conf *model.TaskConfig,
	stop chan bool) error {

	// expand necessary params
	if err := self.expandParams(conf); err != nil {
		return err
	}

	// validate the params
	if err := self.validateParams(); err != nil {
		return fmt.Errorf("expanded params are not valid: %v", err)
	}

	if !self.shouldRunForVariant(conf.BuildVariant.Name) {
		pluginLogger.LogTask(slogger.INFO, "Skipping S3 get of remote file %v for variant %v",
			self.RemoteFile,
			conf.BuildVariant.Name)
		return nil
	}

	// if the local file or extract_to is a relative path, join it to the
	// working dir
	if self.LocalFile != "" && !filepath.IsAbs(self.LocalFile) {
		self.LocalFile = filepath.Join(conf.WorkDir, self.LocalFile)
	}
	if self.ExtractTo != "" && !filepath.IsAbs(self.ExtractTo) {
		self.ExtractTo = filepath.Join(conf.WorkDir, self.ExtractTo)
	}

	errChan := make(chan error)
	go func() {
		errChan <- self.GetWithRetry(pluginLogger)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of S3 Get Command")
		return nil
	}

}

// Wrapper around the Get() function to retry it
func (self *S3GetCommand) GetWithRetry(pluginLogger plugin.Logger) error {
	retriableGet := util.RetriableFunc(
		func() error {
			pluginLogger.LogTask(slogger.INFO, "Fetching %v from"+
				" s3 bucket %v", self.RemoteFile, self.Bucket)
			err := self.Get()
			if err != nil {
				pluginLogger.LogExecution(slogger.ERROR, "Error getting from"+
					" s3 bucket: %v", err)
				return util.RetriableError{err}
			}
			return nil
		},
	)

	retryFail, err := util.RetryArithmeticBackoff(retriableGet,
		MaxS3GetAttempts, S3GetSleep)
	if retryFail {
		pluginLogger.LogExecution(slogger.ERROR, "S3 get failed with error: %v",
			err)
		return err
	}
	return nil
}

// Fetch the specified resource from s3.
func (self *S3GetCommand) Get() error {

	// get the appropriate session and bucket
	auth := &aws.Auth{
		AccessKey: self.AwsKey,
		SecretKey: self.AwsSecret,
	}

	session := thirdparty.NewS3Session(auth, aws.USEast)
	bucket := session.Bucket(self.Bucket)

	// get a reader for the bucket
	reader, err := bucket.GetReader(self.RemoteFile)
	if err != nil {
		return fmt.Errorf("error getting bucket reader for file %v: %v",
			self.RemoteFile, err)
	}
	defer reader.Close()

	// either untar the remote, or just write to a file
	if self.LocalFile != "" {

		// remove the file, if it exists
		exists, err := util.FileExists(self.LocalFile)
		if err != nil {
			return fmt.Errorf("error checking existence of local file %v: %v",
				self.LocalFile, err)
		}
		if exists {
			if err := os.RemoveAll(self.LocalFile); err != nil {
				return fmt.Errorf("error clearing local file %v: %v",
					self.LocalFile, err)
			}
		}

		// open the local file
		file, err := os.Create(self.LocalFile)
		if err != nil {
			return fmt.Errorf("error opening local file %v: %v", self.LocalFile,
				err)
		}
		defer file.Close()

		_, err = io.Copy(file, reader)
		return err

	} else {

		// wrap the reader in a gzip reader and a tar reader
		gzipReader, err := gzip.NewReader(reader)
		if err != nil {
			return fmt.Errorf("error creating gzip reader for %v: %v",
				self.RemoteFile, err)
		}

		tarReader := tar.NewReader(gzipReader)
		err = archive.Extract(tarReader, self.ExtractTo)
		if err != nil {
			return fmt.Errorf("error extracting %v to %v: %v", self.RemoteFile,
				self.ExtractTo, err)
		}
	}

	return nil
}
