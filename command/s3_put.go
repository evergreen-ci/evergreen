package command

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/goamz/goamz/aws"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// A plugin command to put a resource to an s3 bucket and download it to
// the local machine.
type s3put struct {
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

	// ResourceDisplayName stores the name of the file that is linked. Is a prefix when
	// to the matched file name when multiple files are uploaded.
	ResourceDisplayName string `mapstructure:"display_name" plugin:"expand"`

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

	// workDir sets the working directory relative to which s3put should look for files to upload.
	// workDir will be empty if an absolute path is provided to the file.
	workDir string

	taskdata client.TaskData
	base
}

func s3PutFactory() Command      { return &s3put{} }
func (s3pc *s3put) Name() string { return "s3.put" }

// s3put-specific implementation of ParseParams.
func (s3pc *s3put) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, s3pc); err != nil {
		return errors.Wrapf(err, "error decoding %s params", s3pc.Name())
	}

	return s3pc.validate()
}

func (s3pc *s3put) validate() error {
	catcher := grip.NewSimpleCatcher()

	// make sure the command params are valid
	if s3pc.AwsKey == "" {
		catcher.Add(errors.New("aws_key cannot be blank"))
	}
	if s3pc.AwsSecret == "" {
		catcher.Add(errors.New("aws_secret cannot be blank"))
	}
	if s3pc.LocalFile == "" && !s3pc.isMulti() {
		catcher.Add(errors.New("local_file and local_files_include_filter cannot both be blank"))
	}
	if s3pc.LocalFile != "" && s3pc.isMulti() {
		catcher.Add(errors.New("local_file and local_files_include_filter cannot both be specified"))
	}
	if s3pc.Optional && s3pc.isMulti() {
		catcher.Add(errors.New("cannot use optional upload with local_files_include_filter"))
	}
	if s3pc.RemoteFile == "" {
		catcher.Add(errors.New("remote_file cannot be blank"))
	}
	if s3pc.ContentType == "" {
		catcher.Add(errors.New("content_type cannot be blank"))
	}
	if s3pc.isMulti() && filepath.IsAbs(s3pc.LocalFile) {
		catcher.Add(errors.New("cannot use absolute path with local_files_include_filter"))
	}

	if !util.StringSliceContains(artifact.ValidVisibilities, s3pc.Visibility) {
		catcher.Add(errors.Errorf("invalid visibility setting: %v", s3pc.Visibility))
	}

	// make sure the bucket is valid
	if err := validateS3BucketName(s3pc.Bucket); err != nil {
		catcher.Add(errors.Wrapf(err, "%v is an invalid bucket name", s3pc.Bucket))
	}

	// make sure the s3 permissions are valid
	if !validS3Permissions(s3pc.Permissions) {
		catcher.Add(errors.Errorf("permissions '%v' are not valid", s3pc.Permissions))
	}

	return catcher.Resolve()
}

// Apply the expansions from the relevant task config to all appropriate
// fields of the s3put.
func (s3pc *s3put) expandParams(conf *model.TaskConfig) error {
	if filepath.IsAbs(s3pc.LocalFile) {
		s3pc.workDir = ""
	} else {
		s3pc.workDir = conf.WorkDir
	}

	return errors.WithStack(util.ExpandValues(s3pc, conf.Expansions))
}

// isMulti returns whether or not this using the multiple file upload
// capability of the Put command.
func (s3pc *s3put) isMulti() bool {
	return (len(s3pc.LocalFilesIncludeFilter) != 0)
}

func (s3pc *s3put) shouldRunForVariant(buildVariantName string) bool {
	//No buildvariant filter, so run always
	if len(s3pc.BuildVariants) == 0 {
		return true
	}

	//Only run if the buildvariant specified appears in our list.
	return util.StringSliceContains(s3pc.BuildVariants, buildVariantName)
}

// Implementation of Execute.  Expands the parameters, and then puts the
// resource to s3.
func (s3pc *s3put) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	// expand necessary params
	if err := s3pc.expandParams(conf); err != nil {
		return errors.WithStack(err)
	}
	// re-validate command here, in case an expansion is not defined
	if err := s3pc.validate(); err != nil {
		return errors.WithStack(err)
	}

	s3pc.taskdata = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	if !s3pc.shouldRunForVariant(conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 put of local file %v for variant %v",
			s3pc.LocalFile, conf.BuildVariant.Name)
		return nil
	}

	if s3pc.isMulti() {
		logger.Task().Infof("Putting files matching filter %v into path %v in s3 bucket %v",
			s3pc.LocalFilesIncludeFilter, s3pc.RemoteFile, s3pc.Bucket)
	} else {
		logger.Task().Infof("Putting %v into path %v in s3 bucket %v",
			s3pc.LocalFile, s3pc.RemoteFile, s3pc.Bucket)
	}

	errChan := make(chan error)
	go func() {
		errChan <- errors.WithStack(s3pc.putWithRetry(ctx, comm, logger))
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of S3 Put Command")
		return nil
	}

}

// Wrapper around the Put() function to retry it.
func (s3pc *s3put) putWithRetry(ctx context.Context, comm client.Communicator, logger client.LoggerProducer) error {
	backoffCounter := getS3OpBackoff()

	auth := &aws.Auth{
		AccessKey: s3pc.AwsKey,
		SecretKey: s3pc.AwsSecret,
	}

	var (
		err           error
		uploadedFiles []string
	)

	timer := time.NewTimer(0)
	defer timer.Stop()

retryLoop:
	for i := 1; i <= maxS3OpAttempts; i++ {
		logger.Task().Infof("performing s3 put to %s of %s [%d of %d]",
			s3pc.Bucket, s3pc.RemoteFile,
			i, maxS3OpAttempts)

		select {
		case <-ctx.Done():
			return errors.New("s3 put operation canceled")
		case <-timer.C:
			filesList := []string{s3pc.LocalFile}

			if s3pc.isMulti() {
				filesList, err = util.BuildFileList(s3pc.workDir, s3pc.LocalFilesIncludeFilter...)
				if err != nil {
					grip.Error(err)
					return errors.Errorf("could not parse includes filter %s",
						strings.Join(s3pc.LocalFilesIncludeFilter, " "))
				}
			}

			// reset to avoid duplicated uploaded references
			uploadedFiles = []string{}

		uploadLoop:
			for _, fpath := range filesList {
				if ctx.Err() != nil {
					return errors.New("s3 put operation canceled")
				}

				remoteName := s3pc.RemoteFile
				if s3pc.isMulti() {
					fname := filepath.Base(fpath)
					remoteName = fmt.Sprintf("%s%s", s3pc.RemoteFile, fname)
				}

				s3URL := url.URL{
					Scheme: "s3",
					Host:   s3pc.Bucket,
					Path:   remoteName,
				}

				fpath = filepath.Join(s3pc.workDir, fpath)
				err := thirdparty.PutS3File(auth, fpath, s3URL.String(), s3pc.ContentType, s3pc.Permissions)
				if err != nil {
					// retry errors other than "file doesn't exist", which we handle differently based on what
					// kind of upload it is
					if os.IsNotExist(err) {
						if s3pc.isMulti() {
							// try the remaining multi uploads in the group, effectively ignoring this
							// error.
							continue uploadLoop
						} else if s3pc.Optional {
							// single optional file uploads should return early.
							return nil
						} else {
							// single required uploads should return an error asap.
							return errors.Wrapf(err, "missing file %s", fpath)
						}
					}

					// in all other cases, log an error and retry after an interval.
					grip.Error(err)
					timer.Reset(backoffCounter.Duration())
					continue retryLoop
				}

				uploadedFiles = append(uploadedFiles, fpath)
			}

			break retryLoop
		}
	}

	return errors.WithStack(s3pc.attachFiles(ctx, comm, logger, uploadedFiles, s3pc.RemoteFile))
}

// attachTaskFiles is responsible for sending the
// specified file to the API Server. Does not support multiple file putting.
func (s3pc *s3put) attachFiles(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, localFiles []string, remoteFile string) error {
	files := []*artifact.File{}

	for _, fn := range localFiles {
		remoteFileName := filepath.ToSlash(remoteFile)
		if s3pc.isMulti() {
			remoteFileName = fmt.Sprintf("%s%s", remoteFile, filepath.Base(fn))
		}

		fileLink := s3baseURL + s3pc.Bucket + "/" + remoteFileName

		displayName := s3pc.ResourceDisplayName
		if s3pc.isMulti() || displayName == "" {
			displayName = fmt.Sprintf("%s %s", s3pc.ResourceDisplayName, filepath.Base(fn))
		}

		files = append(files, &artifact.File{
			Name:       displayName,
			Link:       fileLink,
			Visibility: s3pc.Visibility,
		})
	}

	err := comm.AttachFiles(ctx, s3pc.taskdata, files)
	if err != nil {
		return errors.Wrap(err, "Attach files failed")
	}

	logger.Execution().Info("API attach files call succeeded")
	return nil
}
