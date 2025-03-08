package command

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	s3Types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/evergreen"
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
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	s3PutAttribute = "evergreen.command.s3_put"
)

var (
	s3PutBucketAttribute               = fmt.Sprintf("%s.bucket", s3PutAttribute)
	s3PutTemporaryCredentialsAttribute = fmt.Sprintf("%s.temporary_credentials", s3PutAttribute)
	s3PutVisibilityAttribute           = fmt.Sprintf("%s.visibility", s3PutAttribute)
	s3PutPermissionsAttribute          = fmt.Sprintf("%s.permissions", s3PutAttribute)
	s3PutRemoteFileAttribute           = fmt.Sprintf("%s.remote_file", s3PutAttribute)
	s3PutExpandedRemoteFileAttribute   = fmt.Sprintf("%s.expanded_remote_file", s3PutAttribute)
	s3PutRoleARN                       = fmt.Sprintf("%s.role_arn", s3PutAttribute)
	s3PutInternalBucket                = fmt.Sprintf("%s.internal_bucket", s3PutAttribute)
)

// s3pc is a command to put a resource to an S3 bucket and download it to
// the local machine.
type s3put struct {
	// AwsKey, AwsSecret, and AwsSessionToken are the user's credentials for
	// authenticating interactions with S3.
	AwsKey          string `mapstructure:"aws_key" plugin:"expand"`
	AwsSecret       string `mapstructure:"aws_secret" plugin:"expand"`
	AwsSessionToken string `mapstructure:"aws_session_token" plugin:"expand"`

	// LocalFile is the local filepath to the file the user
	// wishes to store in S3.
	LocalFile string `mapstructure:"local_file" plugin:"expand"`

	// LocalFilesIncludeFilter is an array of expressions that specify what files should be
	// included in this upload.
	LocalFilesIncludeFilter []string `mapstructure:"local_files_include_filter" plugin:"expand"`

	// LocalFilesIncludeFilterPrefix is an optional path to start processing the LocalFilesIncludeFilter, relative to the working directory.
	LocalFilesIncludeFilterPrefix string `mapstructure:"local_files_include_filter_prefix" plugin:"expand"`

	// RemoteFile is the filepath to store the file to,
	// within an S3 bucket. Is a prefix when multiple files are uploaded via LocalFilesIncludeFilter.
	RemoteFile string `mapstructure:"remote_file" plugin:"expand"`

	// remoteFile is the file path without any expansions applied.
	remoteFile string

	// PreservePath, when set to true, causes multi part uploads uploaded with LocalFilesIncludeFilter to
	// preserve the original folder structure instead of putting all the files into the same folder
	PreservePath string ` mapstructure:"preserve_path" plugin:"expand"`

	// Region is the S3 region where the bucket is located. It defaults to
	// "us-east-1".
	Region string `mapstructure:"region" plugin:"region"`

	// Bucket is the s3 bucket to use when storing the desired file.
	Bucket string `mapstructure:"bucket" plugin:"expand"`

	// Permissions is the ACL to apply to the uploaded file. See:
	// http://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html#canned-acl
	// for some examples.
	Permissions string `mapstructure:"permissions"`

	// ContentType is the MIME type of the uploaded file.
	// E.g. text/html, application/pdf, image/jpeg, ...
	ContentType string `mapstructure:"content_type" plugin:"expand"`

	// BuildVariants stores a list of build variants to run the command for.
	// If the list is empty, it runs for all build variants.
	BuildVariants []string `mapstructure:"build_variants"`

	// ResourceDisplayName stores the name of the file that is linked. Is a prefix when
	// to the matched file name when multiple files are uploaded.
	ResourceDisplayName string `mapstructure:"display_name" plugin:"expand"`

	// Visibility determines who can see file links in the UI.
	// Visibility can be set to either:
	//  "private", which allows logged-in users to see the file;
	//  "public", which allows anyone to see the file; or
	//  "none", which hides the file from the UI for everybody.
	//  "signed" which grants access to private S3 objects to logged-in users
	// If unset, the file will be public.
	Visibility string `mapstructure:"visibility" plugin:"expand"`

	// Optional, when set to true, causes this command to be skipped over without an error when
	// the path specified in local_file does not exist. Defaults to false, which triggers errors
	// for missing files.
	Optional string `mapstructure:"optional" plugin:"expand"`

	// Patchable defaults to true. If set to false, this command will noop without error for patch tasks.
	Patchable string `mapstructure:"patchable" plugin:"patchable"`

	// PatchOnly defaults to false. If set to true, this command will noop without error for non-patch tasks.
	PatchOnly string `mapstructure:"patch_only" plugin:"patch_only"`

	// SkipExisting, when set to 'true', will not upload files if they already
	// exist in s3. This will not cause the s3.put command to fail. This
	// behavior respects s3's strong read-after-write consistency model.
	SkipExisting string `mapstructure:"skip_existing" plugin:"expand"`

	// TemporaryRoleARN is not meant to be used in production. It is used for testing purposes
	// relating to the DEVPROD-5553 project.
	// This is an ARN that should be assumed to make the S3 request.
	// TODO (DEVPROD-13982): Upgrade this flag to RoleARN.
	TemporaryRoleARN string `mapstructure:"temporary_role_arn" plugin:"expand"`

	// workDir sets the working directory relative to which s3put should look for files to upload.
	// workDir will be empty if an absolute path is provided to the file.
	workDir          string
	skipMissing      bool
	preservePath     bool
	skipExistingBool bool
	isPatchable      bool
	isPatchOnly      bool

	bucket          pail.Bucket
	internalBuckets []string

	taskData client.TaskData
	base
}

func s3PutFactory() Command      { return &s3put{} }
func (s3pc *s3put) Name() string { return "s3.put" }

// s3put-specific implementation of ParseParams.
func (s3pc *s3put) ParseParams(params map[string]any) error {
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           s3pc,
	})
	if err != nil {
		return errors.Wrap(err, "initializing mapstructure decoder")
	}

	if err := decoder.Decode(params); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return s3pc.validate()
}

func (s3pc *s3put) validate() error {
	catcher := grip.NewSimpleCatcher()

	if s3pc.TemporaryRoleARN != "" {
		// When using the role ARN, there should be no provided AWS credentials.
		catcher.NewWhen(s3pc.AwsKey != "", "AWS key must be empty when using role ARN")
		catcher.NewWhen(s3pc.AwsSecret != "", "AWS secret must be empty when using role ARN")
		catcher.NewWhen(s3pc.AwsSessionToken != "", "AWS session token must be empty when using role ARN")
	} else {
		catcher.NewWhen(s3pc.AwsKey == "", "AWS key cannot be blank")
		catcher.NewWhen(s3pc.AwsSecret == "", "AWS secret cannot be blank")
	}

	catcher.NewWhen(s3pc.AwsSessionToken != "" && s3pc.Visibility == artifact.Signed, "cannot use temporary AWS credentials with signed link visibility")
	if s3pc.LocalFile == "" && !s3pc.isMulti() {
		catcher.New("local file and local files include filter cannot both be blank")
	}
	if s3pc.LocalFile != "" && s3pc.isMulti() {
		catcher.New("local file and local files include filter cannot both be specified")
	}
	if s3pc.PreservePath != "" && !s3pc.isMulti() {
		catcher.New("preserve path can only be used with local files include filter")
	}
	if s3pc.skipMissing && s3pc.isMulti() {
		catcher.New("cannot use optional with local files include filter as by default it is optional")
	}
	if s3pc.RemoteFile == "" {
		catcher.New("remote file cannot be blank")
	}
	if s3pc.ContentType == "" {
		catcher.New("content type cannot be blank")
	}
	if s3pc.isMulti() && filepath.IsAbs(s3pc.LocalFile) {
		catcher.New("cannot use absolute path with local files include filter")
	}
	if s3pc.Visibility == artifact.Signed && (s3pc.Permissions == string(s3Types.BucketCannedACLPublicRead) || s3pc.Permissions == string(s3Types.BucketCannedACLPublicReadWrite)) {
		catcher.New("visibility: signed should not be combined with permissions: public-read or permissions: public-read-write")
	}

	if !utility.StringSliceContains(artifact.ValidVisibilities, s3pc.Visibility) {
		catcher.Errorf("invalid visibility setting '%s', allowed visibilities are: %s", s3pc.Visibility, artifact.ValidVisibilities)
	}

	catcher.Wrapf(validateS3BucketName(s3pc.Bucket), "invalid bucket name '%s'", s3pc.Bucket)

	// make sure the s3 permissions are valid
	if !validS3Permissions(s3pc.Permissions) {
		catcher.Errorf("invalid permissions '%s'", s3pc.Permissions)
	}

	return catcher.Resolve()
}

// Apply the expansions from the relevant task config
// to all appropriate fields of the s3put.
func (s3pc *s3put) expandParams(conf *internal.TaskConfig) error {
	s3pc.remoteFile = s3pc.RemoteFile

	var err error
	if err = util.ExpandValues(s3pc, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	if s3pc.AwsSessionToken != "" && s3pc.Visibility == artifact.Signed {
		return errors.New("cannot use temporary AWS credentials with a signed link visibility")
	}

	s3pc.workDir = conf.WorkDir
	if filepath.IsAbs(s3pc.LocalFile) {
		s3pc.workDir = ""
	}

	if s3pc.Optional != "" {
		s3pc.skipMissing, err = strconv.ParseBool(s3pc.Optional)
		if err != nil {
			return errors.Wrap(err, "parsing optional parameter as a boolean")
		}
	}

	if s3pc.PreservePath != "" {
		s3pc.preservePath, err = strconv.ParseBool(s3pc.PreservePath)
		if err != nil {
			return errors.Wrap(err, "parsing preserve path parameter as a boolean")
		}
	}

	if s3pc.SkipExisting != "" {
		s3pc.skipExistingBool, err = strconv.ParseBool(s3pc.SkipExisting)
		if err != nil {
			return errors.Wrap(err, "parsing skip existing parameter as a boolean")
		}
	}

	if s3pc.PatchOnly != "" {
		s3pc.isPatchOnly, err = strconv.ParseBool(s3pc.PatchOnly)
		if err != nil {
			return errors.Wrap(err, "parsing patch only parameter as a boolean")
		}
	}

	s3pc.isPatchable = true
	if s3pc.Patchable != "" {
		s3pc.isPatchable, err = strconv.ParseBool(s3pc.Patchable)
		if err != nil {
			return errors.Wrap(err, "parsing patchable parameter as a boolean")
		}
	}

	if s3pc.Region == "" {
		s3pc.Region = evergreen.DefaultEC2Region
	}

	return nil
}

// isMulti returns whether or not this using the multiple file upload
// capability of the Put command.
func (s3pc *s3put) isMulti() bool {
	return (len(s3pc.LocalFilesIncludeFilter) != 0)
}

// Implementation of Execute.  Expands the parameters, and then puts the
// resource to s3.
func (s3pc *s3put) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	s3pc.taskData = client.TaskData{ID: conf.Task.Id, Secret: conf.Task.Secret}
	s3pc.internalBuckets = conf.InternalBuckets

	// expand necessary params
	if err := s3pc.expandParams(conf); err != nil {
		return errors.WithStack(err)
	}
	// re-validate command here, in case an expansion is not defined
	if err := s3pc.validate(); err != nil {
		return errors.Wrap(err, "validating expanded parameters")
	}
	if conf.Task.IsPatchRequest() && !s3pc.isPatchable {
		logger.Task().Infof("Skipping command '%s' because it is not patchable and this task is part of a patch.", s3pc.Name())
		return nil
	}
	if !conf.Task.IsPatchRequest() && s3pc.isPatchOnly {
		logger.Task().Infof("Skipping command '%s' because the command is patch only and this task is not part of a patch.", s3pc.Name())
		return nil
	}

	trace.SpanFromContext(ctx).SetAttributes(
		attribute.String(s3PutBucketAttribute, s3pc.Bucket),
		attribute.Bool(s3PutTemporaryCredentialsAttribute, s3pc.AwsSessionToken != ""),
		attribute.String(s3PutVisibilityAttribute, s3pc.Visibility),
		attribute.String(s3PutPermissionsAttribute, s3pc.Permissions),
		attribute.String(s3PutRemoteFileAttribute, s3pc.remoteFile),
		attribute.String(s3PutExpandedRemoteFileAttribute, s3pc.RemoteFile),
		attribute.String(s3PutRoleARN, s3pc.TemporaryRoleARN),
		attribute.Bool(s3PutInternalBucket, utility.StringSliceContains(s3pc.internalBuckets, s3pc.Bucket)),
	)

	if s3pc.TemporaryRoleARN != "" {
		creds, err := comm.AssumeRole(ctx, s3pc.taskData, apimodels.AssumeRoleRequest{
			RoleARN: s3pc.TemporaryRoleARN,
		})
		if err != nil {
			return errors.Wrap(err, "getting credentials for provided role arn")
		}
		if creds == nil {
			return errors.New("nil credentials returned for provided role arn")
		}
		s3pc.AwsKey = creds.AccessKeyID
		s3pc.AwsSecret = creds.SecretAccessKey
		s3pc.AwsSessionToken = creds.SessionToken
	}

	// create pail bucket
	httpClient := utility.GetHTTPClient()
	httpClient.Timeout = s3HTTPClientTimeout
	defer utility.PutHTTPClient(httpClient)
	if err := s3pc.createPailBucket(ctx, httpClient); err != nil {
		return errors.Wrap(err, "connecting to S3")
	}

	if err := s3pc.bucket.Check(ctx); err != nil {
		return errors.Wrap(err, "checking bucket")
	}

	if !shouldRunForVariant(s3pc.BuildVariants, conf.BuildVariant.Name) {
		logger.Task().Infof("Skipping S3 put of local file '%s' for variant '%s'.",
			s3pc.LocalFile, conf.BuildVariant.Name)
		return nil
	}

	if s3pc.isPrivate(s3pc.Visibility) {
		logger.Task().Infof("Putting private files into S3.")

	} else {
		if s3pc.isMulti() {
			logger.Task().Infof("Putting files matching filter '%s' into path '%s' in S3 bucket '%s'.",
				s3pc.LocalFilesIncludeFilter, s3pc.RemoteFile, s3pc.Bucket)
		} else if s3pc.isPublic() {
			logger.Task().Infof("Putting local file '%s' into path '%s/%s' (%s).", s3pc.LocalFile, s3pc.Bucket, s3pc.RemoteFile, agentutil.S3DefaultURL(s3pc.Bucket, s3pc.RemoteFile))
		} else {
			logger.Task().Infof("Putting local file '%s' into '%s/%s'.", s3pc.LocalFile, s3pc.Bucket, s3pc.RemoteFile)
		}
	}

	errChan := make(chan error)
	go func() {
		err := errors.WithStack(s3pc.putWithRetry(ctx, comm, logger))
		select {
		case errChan <- err:
			return
		case <-ctx.Done():
			logger.Task().Infof("Context canceled waiting for s3 put: %s.", ctx.Err())
			return
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-ctx.Done():
		logger.Execution().Infof("Canceled while running command '%s': %s.", s3pc.Name(), ctx.Err())
		return nil
	}

}

// Wrapper around the Put() function to retry it.
func (s3pc *s3put) putWithRetry(ctx context.Context, comm client.Communicator, logger client.LoggerProducer) error {
	backoffCounter := getS3OpBackoff()

	var (
		err               error
		uploadedFiles     []string
		filesList         []string
		skippedFilesCount int
	)

	timer := time.NewTimer(0)
	defer timer.Stop()

retryLoop:
	for i := 1; i <= maxS3OpAttempts; i++ {
		if s3pc.isPrivate(s3pc.Visibility) {
			logger.Task().Infof("Performing S3 put of a private file.")
		} else {
			logger.Task().Infof("Performing S3 put to file '%s' in bucket '%s' (attempt %d of %d).",
				s3pc.RemoteFile, s3pc.Bucket,
				i, maxS3OpAttempts)
		}

		select {
		case <-ctx.Done():
			return errors.Errorf("canceled while running command '%s'", s3pc.Name())
		case <-timer.C:
			filesList = []string{s3pc.LocalFile}

			if s3pc.isMulti() {
				workDir := filepath.Join(s3pc.workDir, s3pc.LocalFilesIncludeFilterPrefix)
				include := utility.NewGitIgnoreFileMatcher(workDir, s3pc.LocalFilesIncludeFilter...)
				b := utility.FileListBuilder{
					WorkingDir: workDir,
					Include:    include,
				}
				filesList, err = b.Build()
				if err != nil {
					// Skip erroring since local files include filter should treat files as optional.
					if strings.Contains(err.Error(), utility.WalkThroughError) {
						logger.Task().Warningf("Error while building file list: %s", err.Error())
						return nil
					} else {
						return errors.Wrapf(err, "processing local files include filter '%s'",
							strings.Join(s3pc.LocalFilesIncludeFilter, " "))
					}
				}
				if len(filesList) == 0 {
					logger.Task().Warningf("File filter '%s' matched no files.", strings.Join(s3pc.LocalFilesIncludeFilter, " "))
					return nil
				}
			}

			// reset to avoid duplicated uploaded references
			uploadedFiles = []string{}
			skippedFilesCount = 0

		uploadLoop:
			for _, fpath := range filesList {
				if err := ctx.Err(); err != nil {
					return errors.Wrapf(err, "canceled while processing file '%s'", fpath)
				}

				remoteName := s3pc.RemoteFile
				if s3pc.isMulti() {
					if s3pc.preservePath {
						remoteName = filepath.Join(s3pc.RemoteFile, fpath)
					} else {
						// put all files in the same directory
						fname := filepath.Base(fpath)
						remoteName = fmt.Sprintf("%s%s", s3pc.RemoteFile, fname)
					}
				}

				fpath = filepath.Join(filepath.Join(s3pc.workDir, s3pc.LocalFilesIncludeFilterPrefix), fpath)

				err = s3pc.bucket.Upload(ctx, remoteName, fpath)
				if err != nil {
					// retry errors other than "file doesn't exist", which we handle differently based on what
					// kind of upload it is
					if os.IsNotExist(errors.Cause(err)) {
						if s3pc.isMulti() {
							// try the remaining multi uploads in the group, effectively ignoring this
							// error.
							logger.Task().Infof("File '%s' not found, but continuing to upload other files.", fpath)
							continue uploadLoop
						} else if s3pc.skipMissing {
							// single optional file uploads should return early.
							logger.Task().Infof("File '%s' not found and skip missing is true, exiting without error.", fpath)
							return nil
						} else {
							// single required uploads should return an error asap.
							return errors.Wrapf(err, "missing file '%s'", fpath)
						}
					}

					if s3pc.skipExistingBool {
						var ae smithy.APIError

						if errors.As(err, &ae) {
							// This is the error that S3 reports back when
							// the key already exists and we have
							// IfNoneExists set on the put request.
							if ae.ErrorCode() == "PreconditionFailed" {
								skippedFilesCount++

								logger.Task().Infof("Not uploading file '%s' because remote file '%s' already exists. Continuing to upload other files.", fpath, remoteName)
								continue uploadLoop
							}
						}
					}

					// in all other cases, log an error and retry after an interval.
					logger.Task().Error(errors.WithMessage(err, "putting S3 file"))
					timer.Reset(backoffCounter.Duration())
					continue retryLoop
				}

				if s3pc.preservePath {
					uploadedFiles = append(uploadedFiles, remoteName)
				} else {
					uploadedFiles = append(uploadedFiles, fpath)
				}
			}

			break retryLoop
		}
	}

	if len(uploadedFiles) == 0 && s3pc.skipMissing {
		logger.Task().Info("S3 put uploaded no files")
		return nil
	}

	err = errors.WithStack(s3pc.attachFiles(ctx, comm, uploadedFiles, s3pc.RemoteFile))
	if err != nil {
		return err
	}

	logger.Task().WarningWhen(strings.Contains(s3pc.Bucket, "."), "Bucket names containing dots that are created after Sept. 30, 2020 are not guaranteed to have valid attached URLs.")

	processedCount := skippedFilesCount + len(uploadedFiles)

	if processedCount != len(filesList) && !s3pc.skipMissing {
		logger.Task().Infof("Attempted to upload %d files, %d successfully uploaded.", len(filesList), processedCount)
		return errors.Errorf("uploaded %d files of %d requested", processedCount, len(filesList))
	}

	return nil
}

// attachTaskFiles is responsible for sending the
// specified file to the API Server. Does not support multiple file putting.
func (s3pc *s3put) attachFiles(ctx context.Context, comm client.Communicator, localFiles []string, remoteFile string) error {
	files := []*artifact.File{}

	for _, fn := range localFiles {
		remoteFileName := filepath.ToSlash(remoteFile)

		if s3pc.isMulti() {
			if s3pc.preservePath {
				remoteFileName = fn
			} else {
				remoteFileName = fmt.Sprintf("%s%s", remoteFile, filepath.Base(fn))
			}

		}

		fileLink := agentutil.S3DefaultURL(s3pc.Bucket, remoteFileName)

		displayName := s3pc.ResourceDisplayName
		if displayName == "" {
			displayName = filepath.Base(fn)
		} else if s3pc.isMulti() {
			displayName = fmt.Sprintf("%s %s", s3pc.ResourceDisplayName, filepath.Base(fn))
		}
		var key, secret, bucket, fileKey string
		if s3pc.Visibility == artifact.Signed {
			bucket = s3pc.Bucket
			fileKey = remoteFileName
			// If the bucket is an internal one, Evergreen does not need the credentials
			// to sign the URL. If the bucket is not an internal one, Evergreen needs the
			// credentials to sign the URL.
			if !utility.StringSliceContains(s3pc.internalBuckets, s3pc.Bucket) {
				key = s3pc.AwsKey
				secret = s3pc.AwsSecret
			}
		}

		files = append(files, &artifact.File{
			Name:        displayName,
			Link:        fileLink,
			Visibility:  s3pc.Visibility,
			AwsKey:      key,
			AwsSecret:   secret,
			Bucket:      bucket,
			FileKey:     fileKey,
			ContentType: s3pc.ContentType,
		})
	}

	err := comm.AttachFiles(ctx, s3pc.taskData, files)
	if err != nil {
		return errors.Wrap(err, "attaching files")
	}

	return nil
}

func (s3pc *s3put) createPailBucket(ctx context.Context, httpClient *http.Client) error {
	if s3pc.bucket != nil {
		return nil
	}
	opts := pail.S3Options{
		Credentials: pail.CreateAWSCredentials(s3pc.AwsKey, s3pc.AwsSecret, s3pc.AwsSessionToken),
		Region:      s3pc.Region,
		Name:        s3pc.Bucket,
		Permissions: pail.S3Permissions(s3pc.Permissions),
		ContentType: s3pc.ContentType,
	}

	if s3pc.skipExistingBool {
		opts.IfNotExists = true
	}

	bucket, err := pail.NewS3MultiPartBucketWithHTTPClient(ctx, httpClient, opts)
	s3pc.bucket = bucket
	return err
}

func (s3pc *s3put) isPrivate(visibility string) bool {
	if visibility == artifact.Signed || visibility == artifact.Private || visibility == artifact.None {
		return true
	}
	return false
}

func (s3pc *s3put) isPublic() bool {
	return (s3pc.Visibility == "" || s3pc.Visibility == artifact.Public) &&
		(s3pc.Permissions == string(s3Types.BucketCannedACLPublicRead) || s3pc.Permissions == string(s3Types.BucketCannedACLPublicReadWrite))
}
