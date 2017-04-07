package s3

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip/slogger"
	"github.com/pkg/errors"
)

var (
	maxS3PutAttempts = 5
	s3PutSleep       = 5 * time.Second
	s3baseURL        = "https://s3.amazonaws.com/"
)

var errSkippedFile = errors.New("missing optional file was skipped")

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

	// LocalFilesIncludeFilter is an array of expressions that specify what files should be
	// included in this upload.
	LocalFilesIncludeFilter []string `mapstructure:"local_files_include_filter" plugin:"expand"`

	// RemoteFile is the filepath to store the file to,
	// within an s3 bucket. Is a prefix when multiple files are uploaded via LocalFilesIncludeFilter.
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

	// DisplayName stores the name of the file that is linked. Is a prefix when
	// to the matched file name when multiple files are uploaded.
	DisplayName string `mapstructure:"display_name" plugin:"expand"`

	// Visibility determines who can see file links in the UI.
	// Visibility can be set to either
	//  "private", which allows logged-in users to see the file;
	//  "public", which allows anyone to see the file; or
	//  "none", which hides the file from the UI for everybody.
	// If unset, the file will be public.
	Visibility string `mapstructure:"visibility" plugin:"expand"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// the path specified in local_file does not exist. Defaults to false, which triggers errors
	// for missing files.
	Optional bool `mapstructure:"optional"`
}

func (s3pc *S3PutCommand) Name() string {
	return S3PutCmd
}

func (s3pc *S3PutCommand) Plugin() string {
	return S3PluginName
}

// S3PutCommand-specific implementation of ParseParams.
func (s3pc *S3PutCommand) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, s3pc); err != nil {
		return errors.Wrapf(err, "error decoding %s params", s3pc.Name())
	}

	// make sure the command params are valid
	if err := s3pc.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating %s params", s3pc.Name())
	}

	return nil
}

// Validate that all necessary params are set and valid.
func (s3pc *S3PutCommand) validateParams() error {
	if s3pc.AwsKey == "" {
		return errors.New("aws_key cannot be blank")
	}
	if s3pc.AwsSecret == "" {
		return errors.New("aws_secret cannot be blank")
	}
	if s3pc.LocalFile == "" && len(s3pc.LocalFilesIncludeFilter) == 0 {
		return errors.New("local_file and local_files_include_filter cannot both be blank")
	}
	if s3pc.LocalFile != "" && len(s3pc.LocalFilesIncludeFilter) != 0 {
		return errors.New("local_file and local_files_include_filter cannot both be specified")
	}
	if s3pc.Optional && len(s3pc.LocalFilesIncludeFilter) != 0 {
		return errors.New("cannot use optional upload with local_files_include_filter")
	}
	if s3pc.RemoteFile == "" {
		return errors.New("remote_file cannot be blank")
	}
	if s3pc.ContentType == "" {
		return errors.New("content_type cannot be blank")
	}
	if !util.SliceContains(artifact.ValidVisibilities, s3pc.Visibility) {
		return errors.Errorf("invalid visibility setting: %v", s3pc.Visibility)
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(s3pc.Bucket); err != nil {
		return errors.Wrapf(err, "%v is an invalid bucket name", s3pc.Bucket)
	}

	// make sure the s3 permissions are valid
	if !validS3Permissions(s3pc.Permissions) {
		return errors.Errorf("permissions '%v' are not valid", s3pc.Permissions)
	}

	return nil
}

// Apply the expansions from the relevant task config to all appropriate
// fields of the S3PutCommand.
func (s3pc *S3PutCommand) expandParams(conf *model.TaskConfig) error {
	return errors.WithStack(plugin.ExpandValues(s3pc, conf.Expansions))
}

// isMulti returns whether or not this using the multiple file upload
// capability of the Put command.
func (s3pc *S3PutCommand) isMulti() bool {
	return (len(s3pc.LocalFilesIncludeFilter) != 0)
}

func (s3pc *S3PutCommand) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(s3pc.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return util.SliceContains(s3pc.BuildVariants, buildVariantName)
}

// Implementation of Execute.  Expands the parameters, and then puts the
// resource to s3.
func (s3pc *S3PutCommand) Execute(log plugin.Logger,
	com plugin.PluginCommunicator, conf *model.TaskConfig,
	stop chan bool) error {

	// expand necessary params
	if err := s3pc.expandParams(conf); err != nil {
		return errors.WithStack(err)
	}

	// validate the params
	if err := s3pc.validateParams(); err != nil {
		return errors.Wrap(err, "expanded params are not valid")
	}

	if !s3pc.shouldRunForVariant(conf.BuildVariant.Name) {
		log.LogTask(slogger.INFO, "Skipping S3 put of local file %v for variant %v",
			s3pc.LocalFile,
			conf.BuildVariant.Name)
		return nil
	}

	if s3pc.isMulti() {
		log.LogTask(slogger.INFO, "Putting files matching filter %v into path %v in s3 bucket %v",
			s3pc.LocalFilesIncludeFilter, s3pc.RemoteFile, s3pc.Bucket)
	} else {
		if !filepath.IsAbs(s3pc.LocalFile) {
			s3pc.LocalFile = filepath.Join(conf.WorkDir, s3pc.LocalFile)
		}
		log.LogTask(slogger.INFO, "Putting %v into path %v in s3 bucket %v",
			s3pc.LocalFile, s3pc.RemoteFile, s3pc.Bucket)
	}

	errChan := make(chan error)
	go func() {
		errChan <- errors.WithStack(s3pc.PutWithRetry(log, com))
	}()

	select {
	case err := <-errChan:
		return err
	case <-stop:
		log.LogExecution(slogger.INFO, "Received signal to terminate execution of S3 Put Command")
		return nil
	}

}

// Wrapper around the Put() function to retry it.
func (s3pc *S3PutCommand) PutWithRetry(log plugin.Logger, com plugin.PluginCommunicator) error {
	retriablePut := util.RetriableFunc(
		func() error {
			filesList, err := s3pc.Put()
			if err != nil {
				if err == errSkippedFile {
					return errors.WithStack(err)
				}
				log.LogExecution(slogger.ERROR, "Error putting to s3 bucket: %v", err)
				return util.RetriableError{err}
			}

			for _, file := range filesList {
				remoteName := s3pc.RemoteFile
				if s3pc.isMulti() {
					fname := filepath.Base(file)
					remoteName = fmt.Sprintf("%s%s", s3pc.RemoteFile, fname)
				}

				err = s3pc.AttachTaskFiles(log, com, file, remoteName)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		},
	)

	retryFail, err := util.RetryArithmeticBackoff(retriablePut, maxS3PutAttempts, s3PutSleep)
	if err == errSkippedFile {
		log.LogExecution(slogger.INFO, "S3 put skipped optional missing file.")
		return nil
	}
	if retryFail {
		log.LogExecution(slogger.ERROR, "S3 put failed with error: %v", err)
		return errors.WithStack(err)
	}

	return nil
}

// Put the specified resource to s3.
func (s3pc *S3PutCommand) Put() ([]string, error) {
	var err error

	filesList := []string{s3pc.LocalFile}

	if s3pc.isMulti() {
		filesList, err = util.BuildFileList(".", s3pc.LocalFilesIncludeFilter...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	for _, fpath := range filesList {
		remoteName := s3pc.RemoteFile
		if s3pc.isMulti() {
			fname := filepath.Base(fpath)
			remoteName = fmt.Sprintf("%s%s", s3pc.RemoteFile, fname)
		}

		auth := &aws.Auth{
			AccessKey: s3pc.AwsKey,
			SecretKey: s3pc.AwsSecret,
		}
		s3URL := url.URL{
			Scheme: "s3",
			Host:   s3pc.Bucket,
			Path:   remoteName,
		}
		err := thirdparty.PutS3File(auth, fpath, s3URL.String(), s3pc.ContentType, s3pc.Permissions)
		if err != nil {
			if !s3pc.isMulti() {
				if s3pc.Optional && os.IsNotExist(err) {
					// important to *not* wrap this error.
					return nil, errSkippedFile
				}
			}
			return nil, errors.WithStack(err)
		}
	}
	return filesList, nil
}

// AttachTaskFiles is responsible for sending the
// specified file to the API Server. Does not support multiple file putting.
func (s3pc *S3PutCommand) AttachTaskFiles(log plugin.Logger,
	com plugin.PluginCommunicator, localFile, remoteFile string) error {

	remoteFileName := filepath.ToSlash(remoteFile)
	if s3pc.isMulti() {
		remoteFileName = fmt.Sprintf("%s%s", remoteFile, filepath.Base(localFile))
	}
	fileLink := s3baseURL + s3pc.Bucket + "/" + remoteFileName

	displayName := s3pc.DisplayName
	if s3pc.isMulti() || displayName == "" {
		displayName = fmt.Sprintf("%s%s", s3pc.DisplayName, filepath.Base(localFile))
	}

	file := &artifact.File{
		Name:       displayName,
		Link:       fileLink,
		Visibility: s3pc.Visibility,
	}

	err := com.PostTaskFiles([]*artifact.File{file})
	if err != nil {
		return errors.Wrap(err, "Attach files failed")
	}
	log.LogExecution(slogger.INFO, "API attach files call succeeded")
	return nil
}
