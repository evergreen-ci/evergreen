package command

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	pushLogSuccess = "success"
	pushLogFailed  = "failed"
)

// The S3CopyPlugin consists of zero or more files that are to be copied
// from one location in S3 to the other.
type s3copy struct {
	// AwsKey and AwsSecret are the user's credentials for
	// authenticating interactions with s3.
	AwsKey    string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret string `mapstructure:"aws_secret" plugin:"expand"`
	// An array of file copy configurations
	S3CopyFiles []*s3CopyFile `mapstructure:"s3_copy_files" plugin:"expand"`

	// Bucket is the s3 bucket to use when storing the desired file
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// Permissions is the ACL to apply to the uploaded file. See:
	//  http://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl
	// for some examples.
	Permissions string `mapstructure:"permissions"`

	// ContentType is the MIME type of the uploaded file.
	//  E.g. text/html, application/pdf, image/jpeg, ...
	ContentType string `mapstructure:"content_type" plugin:"expand"`

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

	// Permissions is the ACL to apply to the copied file. See:
	//  http://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl
	// for some examples.
	Permissions string `mapstructure:"permissions" plugin:"expand"`

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
	// Region is the s3 region where the bucket is located. It defaults to
	// "us-east-1".
	Region string `mapstructure:"region" plugin:"region"`

	// Bucket is the s3 bucket for the file.
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// Path is the file path within the bucket.
	Path string `mapstructure:"path" plugin:"expand"`
}

func s3CopyFactory() Command   { return &s3copy{} }
func (c *s3copy) Name() string { return "s3Copy.copy" }

// ParseParams decodes the S3 push command parameters
func (c *s3copy) ParseParams(params map[string]interface{}) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           c,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if err := decoder.Decode(params); err != nil {
		return errors.Wrapf(err, "error decoding %s params", c.Name())
	}

	return c.validate()
}

// validateParams is a helper function that ensures all
// the fields necessary for carrying out an S3 copy operation are present
func (c *s3copy) validate() error {
	catcher := grip.NewSimpleCatcher()

	// make sure the command params are valid
	if c.AwsKey == "" {
		catcher.New("aws_key cannot be blank")
	}
	if c.AwsSecret == "" {
		catcher.New("aws_secret cannot be blank")
	}

	for _, s3CopyFile := range c.S3CopyFiles {
		if s3CopyFile.Source.Path == "" {
			catcher.New("s3 source path cannot be blank")
		}
		if s3CopyFile.Destination.Path == "" {
			catcher.New("s3 destination path cannot be blank")
		}
		if s3CopyFile.Permissions == "" {
			s3CopyFile.Permissions = s3.BucketCannedACLPublicRead
		}
		if s3CopyFile.Source.Region == "" {
			s3CopyFile.Source.Region = endpoints.UsEast1RegionID
		}
		if s3CopyFile.Destination.Region == "" {
			s3CopyFile.Destination.Region = endpoints.UsEast1RegionID
		}
		// make sure both buckets are valid
		if err := validateS3BucketName(s3CopyFile.Source.Bucket); err != nil {
			catcher.Wrapf(err, "source bucket '%v' is invalid", s3CopyFile.Source.Bucket)
		}
		if err := validateS3BucketName(s3CopyFile.Destination.Bucket); err != nil {
			catcher.Wrapf(err, "destination bucket '%v' is invalid", s3CopyFile.Destination.Bucket)
		}

	}

	return catcher.Resolve()
}

// Implementation of Execute.  Expands the parameters, and then copies the
// resource from one s3 bucket to another one.
func (c *s3copy) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	// expand necessary params
	if err := util.ExpandValues(c, conf.Expansions); err != nil {
		return errors.WithStack(err)
	}
	// re-validate command here, in case an expansion is not defined
	if err := c.validate(); err != nil {
		return errors.WithStack(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errChan := make(chan error)
	go func() {
		errChan <- errors.WithStack(c.copyWithRetry(ctx, comm, logger, conf))
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		logger.Execution().Info("Received signal to terminate execution of S3 copy command")
		return nil
	}

}

func (c *s3copy) copyWithRetry(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	backoffCounter := getS3OpBackoff()
	td := client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}

	var (
		foundDottedBucketName bool
		uploadedFiles         int
	)

	client := utility.GetHTTPClient()
	client.Timeout = 10 * time.Minute
	defer utility.PutHTTPClient(client)

	timer := time.NewTimer(0)
	defer timer.Stop()

retryLoop:
	for i := 1; i <= maxS3OpAttempts; i++ {

		select {
		case <-ctx.Done():
			return errors.New("s3 copy operation canceled")
		case <-timer.C:
			logger.Task().Infof("copying files: (attempt %d of %d)",
				i, maxS3OpAttempts)

		uploadLoop:
			for _, s3CopyFile := range c.S3CopyFiles {
				if ctx.Err() != nil {
					return errors.New("s3 copy operation canceled")
				}

				s3CopyReq := apimodels.S3CopyRequest{
					S3SourceRegion:      s3CopyFile.Source.Region,
					S3SourceBucket:      s3CopyFile.Source.Bucket,
					S3SourcePath:        s3CopyFile.Source.Path,
					S3DestinationRegion: s3CopyFile.Destination.Region,
					S3DestinationBucket: s3CopyFile.Destination.Bucket,
					S3DestinationPath:   s3CopyFile.Destination.Path,
					S3DisplayName:       s3CopyFile.DisplayName,
					S3Permissions:       s3CopyFile.Permissions,
				}
				newPushLog, err := comm.NewPush(ctx, td, &s3CopyReq)
				if err != nil {
					return errors.Wrap(err, "error adding pushlog")
				}
				if newPushLog.TaskId == "" {
					logger.Task().Infof("noop, this version is currently in the process of trying to push, or has already succeeded in pushing the file: '%s/%s'", s3CopyFile.Destination.Bucket, s3CopyFile.Destination.Path)
					continue
				}

				if len(s3CopyFile.BuildVariants) > 0 && !utility.StringSliceContains(
					s3CopyFile.BuildVariants, conf.BuildVariant.Name) {
					continue
				}

				logger.Execution().Infof("Making API push copy call to "+
					"transfer %v/%v => %v/%v", s3CopyFile.Source.Bucket,
					s3CopyFile.Source.Path, s3CopyFile.Destination.Bucket,
					s3CopyFile.Destination.Path)

				s3CopyReq.AwsKey = c.AwsKey
				s3CopyReq.AwsSecret = c.AwsSecret

				// Now copy the file into the permanent location
				srcOpts := pail.S3Options{
					Credentials: pail.CreateAWSCredentials(s3CopyReq.AwsKey, s3CopyReq.AwsSecret, ""),
					Region:      s3CopyReq.S3SourceRegion,
					Name:        s3CopyReq.S3SourceBucket,
					Permissions: pail.S3Permissions(s3CopyReq.S3Permissions),
				}

				srcBucket, err := pail.NewS3MultiPartBucketWithHTTPClient(client, srcOpts)
				if err != nil {
					connectionErr := errors.Wrap(err, "S3 copy failed, could not establish connection to source bucket")
					logger.Task().Error(connectionErr)
					return connectionErr
				}

				if err := srcBucket.Check(ctx); err != nil {
					return errors.Wrap(err, "invalid bucket")
				}
				destOpts := pail.S3Options{
					Credentials: pail.CreateAWSCredentials(s3CopyReq.AwsKey, s3CopyReq.AwsSecret, ""),
					Region:      s3CopyReq.S3DestinationRegion,
					Name:        s3CopyReq.S3DestinationBucket,
					Permissions: pail.S3Permissions(s3CopyReq.S3Permissions),
				}
				destBucket, err := pail.NewS3MultiPartBucket(destOpts)
				if err != nil {
					connectionErr := errors.Wrap(err, "S3 copy failed, could not establish connection to destination bucket")
					logger.Task().Error(connectionErr)
					return connectionErr
				}
				copyOpts := pail.CopyOptions{
					SourceKey:         s3CopyReq.S3SourcePath,
					DestinationKey:    s3CopyReq.S3DestinationPath,
					DestinationBucket: destBucket,
				}
				err = srcBucket.Copy(ctx, copyOpts)

				if err != nil {
					newPushLog.Status = pushLogFailed
					if err := comm.UpdatePushStatus(ctx, td, newPushLog); err != nil {
						return errors.Wrap(err, "updating pushlog status failed for task")
					}
					if s3CopyFile.Optional {
						logger.Execution().Errorf("S3 push copy failed to copy '%s' to '%s'. File is optional, continuing \n error: %v",
							s3CopyFile.Source.Path, s3CopyFile.Destination.Bucket, err)
						continue uploadLoop
					}
					// in all other cases, log an error and retry after an interval.
					logger.Execution().Errorf("S3 push copy failed to copy '%s' to '%s'. File is not optional, will retry \n error: %v",
						s3CopyFile.Source.Path, s3CopyFile.Destination.Bucket, err)
					timer.Reset(backoffCounter.Duration())
					continue retryLoop
				}
				uploadedFiles += 1
				newPushLog.Status = pushLogSuccess
				if err := comm.UpdatePushStatus(ctx, td, newPushLog); err != nil {
					return errors.Wrap(err, "updating pushlog status success for task")
				}
				if err = c.attachFiles(ctx, comm, logger, td, s3CopyReq); err != nil {
					return errors.WithStack(err)
				}

				if !foundDottedBucketName && strings.Contains(s3CopyReq.S3DestinationBucket, ".") {
					logger.Task().Warning("destination bucket names containing dots that are created after Sept. 30, 2020 are not guaranteed to have valid attached URLs")
					foundDottedBucketName = true
				}
				logger.Task().Infof("successfully copied '%s' to '%s'", s3CopyFile.Source.Path, s3CopyFile.Destination.Path)
			}

			break retryLoop
		}
	}

	logger.Task().Infof("attempted to upload %d files, %d successfully uploaded", len(c.S3CopyFiles), uploadedFiles)
	return nil
}

// attachFiles is responsible for sending the
// specified file to the API Server
func (c *s3copy) attachFiles(ctx context.Context, comm client.Communicator,
	logger client.LoggerProducer, td client.TaskData, request apimodels.S3CopyRequest) error {

	remotePath := filepath.ToSlash(request.S3DestinationPath)
	fileLink := agentutil.S3DefaultURL(request.S3DestinationBucket, remotePath)
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
