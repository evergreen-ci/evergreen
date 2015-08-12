package s3Plugin

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/goamz/goamz/s3"
	"github.com/mitchellh/mapstructure"
	"os"
	"path/filepath"
	"time"
)

var (
	maxS3PutAttempts           = 5
	s3PutSleep                 = 5 * time.Second
	attachResultsPostRetries   = 5
	attachResultsRetrySleepSec = 10 * time.Second
	s3baseURL                  = "https://s3.amazonaws.com/"
)

// A plugin command to put a resource to an s3 bucket and download it to
// the local machine.
type S3PutCommand struct {
	// AwsKey and AwsSecret are the user's credentials for
	// authenticating interactions with s3.
	AwsKey    string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand"`

	// LocalFile is the local filepath to the file the user
	// wishes to store in s3
	LocalFile string `mapstructure:"local_file" plugin:"expand"`

	// RemoteFile is the filepath to store the file to,
	// within an s3 bucket
	RemoteFile string `mapstructure:"remote_file" plugin:"expand"`

	// Bucket is the s3 bucket to use when storing the desired file
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// Permission is the ACL to apply to the uploaded file. See:
	//  http://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl
	// for some examples.
	Permissions string `mapstructure:"permissions"`

	// ContentType is the MIME type of the uploaded file.
	//  E.g. text/html, application/pdf, image/jpeg, ...
	ContentType string `mapstructure:"content_type" plugin:"expand"`

	// BuildVariants stores a list of MCI build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants"`

	// DisplayName stores the name of the file that is linked
	DisplayName string `mapstructure:"display_name" plugin:"expand"`

	// Visibility determines who can see file links in the UI.
	// Visibility can be set to either
	//  "private", which allows logged-in users to see the file;
	//  "public", which allows anyone to see the file; or
	//  "none", which hides the file from the UI for everybody.
	// If unset, the file will be public.
	Visibility string `mapstructure:"visibility" plugin:"expand"`
}

func (self *S3PutCommand) Name() string {
	return S3PutCmd
}

func (self *S3PutCommand) Plugin() string {
	return S3PluginName
}

// S3PutCommand-specific implementation of ParseParams.
func (self *S3PutCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, self); err != nil {
		return fmt.Errorf("error decoding %v params: %v", self.Name(), err)
	}

	// make sure the command params are valid
	if err := self.validateParams(); err != nil {
		return fmt.Errorf("error validating %v params: %v", self.Name(), err)
	}

	return nil

}

// Validate that all necessary params are set and valid.
func (self *S3PutCommand) validateParams() error {
	if self.AwsKey == "" {
		return fmt.Errorf("aws_key cannot be blank")
	}
	if self.AwsSecret == "" {
		return fmt.Errorf("aws_secret cannot be blank")
	}
	if self.LocalFile == "" {
		return fmt.Errorf("local_file cannot be blank")
	}
	if self.RemoteFile == "" {
		return fmt.Errorf("remote_file cannot be blank")
	}
	if self.ContentType == "" {
		return fmt.Errorf("content_type cannot be blank")
	}
	if !util.SliceContains(artifact.ValidVisibilities, self.Visibility) {
		return fmt.Errorf("invalid visibility setting: %v", self.Visibility)
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(self.Bucket); err != nil {
		return fmt.Errorf("%v is an invalid bucket name: %v", self.Bucket, err)
	}

	// make sure the s3 permissions are valid
	if !validS3Permissions(self.Permissions) {
		return fmt.Errorf("permissions '%v' are not valid", self.Permissions)
	}

	return nil
}

// Apply the expansions from the relevant task config to all appropriate
// fields of the S3PutCommand.
func (self *S3PutCommand) expandParams(conf *model.TaskConfig) error {
	return plugin.ExpandValues(self, conf.Expansions)
}

func (self *S3PutCommand) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(self.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return util.SliceContains(self.BuildVariants, buildVariantName)
}

// Implementation of Execute.  Expands the parameters, and then puts the
// resource to s3.
func (self *S3PutCommand) Execute(pluginLogger plugin.Logger,
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
		pluginLogger.LogTask(slogger.INFO, "Skipping S3 put of local file %v for variant %v",
			self.LocalFile,
			conf.BuildVariant.Name)
		return nil
	}

	// if the local file is a relative path, join it to the work dir
	if !filepath.IsAbs(self.LocalFile) {
		self.LocalFile = filepath.Join(conf.WorkDir, self.LocalFile)
	}

	pluginLogger.LogTask(slogger.INFO, "Putting %v into path %v in s3 bucket %v",
		self.LocalFile, self.RemoteFile, self.Bucket)

	errChan := make(chan error)
	go func() {
		errChan <- self.PutWithRetry(pluginLogger, pluginCom)
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		pluginLogger.LogExecution(slogger.INFO, "Received signal to terminate"+
			" execution of S3 Put Command")
		return nil
	}

}

// Wrapper around the Put() function to retry it
func (self *S3PutCommand) PutWithRetry(pluginLogger plugin.Logger,
	pluginComm plugin.PluginCommunicator) error {
	retriablePut := util.RetriableFunc(
		func() error {
			err := self.Put()
			if err != nil {
				pluginLogger.LogExecution(slogger.ERROR, "Error putting to s3"+
					" bucket: %v", err)
				return util.RetriableError{err}
			}
			return nil
		},
	)

	retryFail, err := util.RetryArithmeticBackoff(retriablePut,
		maxS3PutAttempts, s3PutSleep)
	if retryFail {
		pluginLogger.LogExecution(slogger.ERROR, "S3 put failed with error: %v",
			err)
		return err
	}
	return self.AttachTaskFiles(pluginLogger, pluginComm)
}

// Put the specified resource to s3.
func (self *S3PutCommand) Put() error {

	fi, err := os.Stat(self.LocalFile)
	if err != nil {
		return err
	}

	fileReader, err := os.Open(self.LocalFile)
	if err != nil {
		return err
	}
	defer fileReader.Close()

	// get the appropriate session and bucket
	auth := &aws.Auth{
		AccessKey: self.AwsKey,
		SecretKey: self.AwsSecret,
	}
	session := thirdparty.NewS3Session(auth, aws.USEast)

	bucket := session.Bucket(self.Bucket)

	options := s3.Options{}
	// put the data
	return bucket.PutReader(
		self.RemoteFile,
		fileReader,
		fi.Size(),
		self.ContentType,
		s3.ACL(self.Permissions),
		options,
	)

}

// AttachTaskFiles is responsible for sending the
// specified file to the API Server
func (self *S3PutCommand) AttachTaskFiles(pluginLogger plugin.Logger,
	pluginCom plugin.PluginCommunicator) error {

	remoteFile := filepath.ToSlash(self.RemoteFile)
	fileLink := s3baseURL + self.Bucket + "/" + remoteFile

	displayName := self.DisplayName
	if displayName == "" {
		displayName = filepath.Base(self.LocalFile)
	}
	file := &artifact.File{
		Name:       displayName,
		Link:       fileLink,
		Visibility: self.Visibility,
	}

	err := pluginCom.PostTaskFiles([]*artifact.File{file})
	if err != nil {
		return fmt.Errorf("Attach files failed: %v", err)
	}
	pluginLogger.LogExecution(slogger.INFO, "API attach files call succeeded")
	return nil
}
