package command

import (
	"path/filepath"

	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
)

// The S3CopyPlugin consists of zero or more files that are to be copied
// from one location in S3 to the other.
type s3copy struct {
	// AwsKey & AwsSecret are provided to make it possible to transfer
	// files to/from any bucket using the appropriate keys for each
	AwsKey    string `mapstructure:"aws_key" plugin:"expand" json:"aws_key"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand" json:"aws_secret"`

	// An array of file copy configurations
	S3CopyFiles []*s3CopyFile `mapstructure:"s3_copy_files" plugin:"expand"`
	base
}

type s3CopyFile struct {
	// Each source and destination is specified in the
	// following manner:
	//  bucket: <s3 bucket>
	//  path: <path to file>
	//
	// e.g.
	//  bucket: mciuploads
	//  path: linux-64/x86_64/artifact.tgz
	Source      s3Loc `mapstructure:"source" plugin:"expand"`
	Destination s3Loc `mapstructure:"destination" plugin:"expand"`

	// BuildVariants is a slice of build variants for which
	// a specified file is to be copied. An empty slice indicates it is to be
	// copied for all build variants
	BuildVariants []string `mapstructure:"build_variants" plugin:"expand"`

	//DisplayName is the name of the file
	DisplayName string `mapstructure:"display_name" plugin:"expand"`

	// Optional, when true suppresses the error state for the file.
	Optional bool `mapstructure:"optional"`
}

// s3Loc is a format for describing the location of a file in
// Amazon's S3. It contains an entry for the bucket name and another
// describing the path name of the file within the bucket
type s3Loc struct {
	// the s3 bucket for the file
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// the file path within the bucket
	Path string `mapstructure:"path" plugin:"expand"`
}

func s3CopyFactory() Command   { return &s3copy{} }
func (c *s3copy) Name() string { return "s3Copy.copy" }

// ParseParams decodes the S3 push command parameters that are
// specified as part of an S3CopyPlugin command; this is required
// to satisfy the 'Command' interface
func (c *s3copy) ParseParams(params map[string]interface{}) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrapf(err, "error decoding %v params", c.Name())
	}
	if err := c.validateParams(); err != nil {
		return errors.Wrapf(err, "error validating %v params", c.Name())
	}
	return nil
}

// validateParams is a helper function that ensures all
// the fields necessary for carrying out an S3 copy operation are present
func (c *s3copy) validateParams() error {
	if c.AwsKey == "" {
		return errors.New("s3 AWS key cannot be blank")
	}
	if c.AwsSecret == "" {
		return errors.New("s3 AWS secret cannot be blank")
	}
	for _, s3CopyFile := range c.S3CopyFiles {
		if s3CopyFile.Source.Bucket == "" {
			return errors.New("s3 source bucket cannot be blank")
		}
		if s3CopyFile.Destination.Bucket == "" {
			return errors.New("s3 destination bucket cannot be blank")
		}
		if s3CopyFile.Source.Path == "" {
			return errors.New("s3 source path cannot be blank")
		}
		if s3CopyFile.Destination.Path == "" {
			return errors.New("s3 destination path cannot be blank")
		}

		err := validateS3BucketName(s3CopyFile.Source.Bucket)
		if err != nil {
			return errors.Wrapf(err, "source bucket '%v' is invalid",
				s3CopyFile.Source.Bucket)
		}

		err = validateS3BucketName(s3CopyFile.Destination.Bucket)
		if err != nil {
			return errors.Wrapf(err, "destination bucket '%v' is invalid",
				s3CopyFile.Destination.Bucket)
		}
	}

	return nil
}

// Execute carries out the s3copy command - this is required
// to satisfy the 'Command' interface
func (c *s3copy) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	// expand the S3 copy parameters before running the task
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.WithStack(err)
	}

	errChan := make(chan error)
	go func() {
		errChan <- errors.WithStack(c.s3Copy(ctx, comm, logger, conf))
	}()

	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of S3 copy command")
		return nil
	}
}

// S3Copy is responsible for carrying out the core of the S3CopyPlugin's
// function - it makes an API calls to copy a given staged file to it's final
// production destination
func (c *s3copy) s3Copy(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	for _, s3CopyFile := range c.S3CopyFiles {
		if len(s3CopyFile.BuildVariants) > 0 && !util.SliceContains(
			s3CopyFile.BuildVariants, conf.BuildVariant.Name) {
			continue
		}

		if ctx.Err() != nil {
			return errors.New("s3copy operation received was canceled")
		}

		logger.Execution().Infof("Making API push copy call to "+
			"transfer %v/%v => %v/%v", s3CopyFile.Source.Bucket,
			s3CopyFile.Source.Path, s3CopyFile.Destination.Bucket,
			s3CopyFile.Destination.Path)

		s3CopyReq := apimodels.S3CopyRequest{
			AwsKey:              c.AwsKey,
			AwsSecret:           c.AwsSecret,
			S3SourceBucket:      s3CopyFile.Source.Bucket,
			S3SourcePath:        s3CopyFile.Source.Path,
			S3DestinationBucket: s3CopyFile.Destination.Bucket,
			S3DestinationPath:   s3CopyFile.Destination.Path,
			S3DisplayName:       s3CopyFile.DisplayName,
		}

		err := comm.S3Copy(ctx, td, &s3CopyReq)
		if err != nil {
			err = errors.Wrap(err, "s3 push copy failed")
			logger.Execution().Error(err)

			if s3CopyFile.Optional {
				logger.Execution().Errorf("file '%s' is optional, continuing",
					s3CopyFile.DisplayName)
				continue
			}

		}

		err = c.attachFiles(ctx, comm, logger, td, s3CopyReq)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

// AttachTaskFiles is responsible for sending the
// specified file to the API Server
func (c *s3copy) attachFiles(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, td client.TaskData, request apimodels.S3CopyRequest) error {

	remotePath := filepath.ToSlash(request.S3DestinationPath)
	fileLink := s3baseURL + request.S3DestinationBucket + "/" + remotePath

	displayName := request.S3DisplayName

	if displayName == "" {
		displayName = filepath.Base(request.S3SourcePath)
	}

	logger.Execution().Infof("attaching file with name %v", displayName)

	file := artifact.File{
		Name: displayName,
		Link: fileLink,
	}

	files := []*artifact.File{&file}

	if err := comm.AttachFiles(ctx, td, files); err != nil {
		return errors.Wrap(err, "Attach files failed")
	}

	logger.Execution().Info("API attach files call succeeded")
	return nil
}
