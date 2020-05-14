package pail

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const compressionEncoding = "gzip"

// S3Permissions is a type that describes the object canned ACL from S3.
type S3Permissions string

// Valid S3 permissions.
const (
	S3PermissionsPrivate                S3Permissions = s3.ObjectCannedACLPrivate
	S3PermissionsPublicRead             S3Permissions = s3.ObjectCannedACLPublicRead
	S3PermissionsPublicReadWrite        S3Permissions = s3.ObjectCannedACLPublicReadWrite
	S3PermissionsAuthenticatedRead      S3Permissions = s3.ObjectCannedACLAuthenticatedRead
	S3PermissionsAWSExecRead            S3Permissions = s3.ObjectCannedACLAwsExecRead
	S3PermissionsBucketOwnerRead        S3Permissions = s3.ObjectCannedACLBucketOwnerRead
	S3PermissionsBucketOwnerFullControl S3Permissions = s3.ObjectCannedACLBucketOwnerFullControl
)

// Validate s3 permissions.
func (p S3Permissions) Validate() error {
	switch p {
	case S3PermissionsPublicRead, S3PermissionsPublicReadWrite:
		return nil
	case S3PermissionsPrivate, S3PermissionsAuthenticatedRead, S3PermissionsAWSExecRead:
		return nil
	case S3PermissionsBucketOwnerRead, S3PermissionsBucketOwnerFullControl:
		return nil
	default:
		return errors.New("invalid S3 permissions type specified")
	}
}

type s3BucketSmall struct {
	s3Bucket
}

type s3BucketLarge struct {
	s3Bucket
	minPartSize int
}

type s3Bucket struct {
	dryRun              bool
	deleteOnPush        bool
	deleteOnPull        bool
	singleFileChecksums bool
	compress            bool
	verbose             bool
	batchSize           int
	sess                *session.Session
	svc                 *s3.S3
	name                string
	prefix              string
	permissions         S3Permissions
	contentType         string
}

// S3Options support the use and creation of S3 backed buckets.
type S3Options struct {
	// DryRun enables running in a mode that will not execute any
	// operations that modify the bucket.
	DryRun bool
	// DeleteOnSync will delete all objects from the target that do not
	// exist in the destination after the completion of a sync operation
	// (Push/Pull).
	DeleteOnSync bool
	// DeleteOnPush will delete all objects from the target that do not
	// exist in the source after the completion of Push.
	DeleteOnPush bool
	// DeleteOnPull will delete all objects from the target that do not
	// exist in the source after the completion of Pull.
	DeleteOnPull bool
	// Compress enables gzipping of uploaded objects.
	Compress bool
	// UseSingleFileChecksums forces the bucket to checksum files before
	// running uploads and download operation (rather than doing these
	// operations independently.) Useful for large files, particularly in
	// coordination with the parallel sync bucket implementations.
	UseSingleFileChecksums bool
	// Verbose sets the logging mode to "debug".
	Verbose bool
	// MaxRetries sets the number of retry attempts for s3 operations.
	MaxRetries int
	// Credentials allows the passing in of explicit AWS credentials. These
	// will override the default credentials chain. (Optional)
	Credentials *credentials.Credentials
	// SharedCredentialsFilepath, when not empty, will override the default
	// credentials chain and the Credentials value (see above). (Optional)
	SharedCredentialsFilepath string
	// SharedCredentialsProfile, when not empty, will temporarily set the
	// AWS_PROFILE environment variable to its value. (Optional)
	SharedCredentialsProfile string
	// Region specifies the AWS region.
	Region string
	// Name specifies the name of the bucket.
	Name string
	// Prefix specifies the prefix to use. (Optional)
	Prefix string
	// Permissions sets the S3 permissions to use for each object. Defaults
	// to FULL_CONTROL. See
	// `https://docs.aws.amazon.com/AmazonS3/latest/dev/acl-overview.html`
	// for more information.
	Permissions S3Permissions
	// ContentType sets the standard MIME type of the objet data. Defaults
	// to nil. See
	//`https://www.w3.org/Protocols/rfc2616/rfc2616-sec14.html#sec14.17`
	// for more information.
	ContentType string
}

// CreateAWSCredentials is a wrapper for creating AWS credentials.
func CreateAWSCredentials(awsKey, awsPassword, awsToken string) *credentials.Credentials {
	return credentials.NewStaticCredentials(awsKey, awsPassword, awsToken)
}

func (s *s3Bucket) normalizeKey(key string) string {
	if key == "" {
		return s.prefix
	}
	return consistentJoin(s.prefix, key)
}

func (s *s3Bucket) denormalizeKey(key string) string {
	if s.prefix != "" && len(key) > len(s.prefix)+1 {
		key = key[len(s.prefix)+1:]
	}
	return key
}

func newS3BucketBase(client *http.Client, options S3Options) (*s3Bucket, error) {
	if options.Permissions != "" {
		if err := options.Permissions.Validate(); err != nil {
			return nil, errors.WithStack(err)
		}
	}

	if (options.DeleteOnPush != options.DeleteOnPull) && options.DeleteOnSync {
		return nil, errors.New("ambiguous delete on sync options set")
	}

	config := &aws.Config{
		Region:     aws.String(options.Region),
		HTTPClient: client,
		MaxRetries: aws.Int(options.MaxRetries),
	}

	if options.SharedCredentialsProfile != "" {
		prev := os.Getenv("AWS_PROFILE")
		if err := os.Setenv("AWS_PROFILE", options.SharedCredentialsProfile); err != nil {
			return nil, errors.Wrap(err, "problem setting AWS_PROFILE env var")
		}
		defer func() {
			if err := os.Setenv("AWS_PROFILE", prev); err != nil {
				grip.Error(errors.Wrap(err, "problem setting back AWS_PROFILE env var"))
			}
		}()
	}
	if options.SharedCredentialsFilepath != "" {
		sharedCredentials := credentials.NewSharedCredentials(options.SharedCredentialsFilepath, options.SharedCredentialsProfile)
		_, err := sharedCredentials.Get()
		if err != nil {
			return nil, errors.Wrapf(err, "invalid credentials from profile '%s'", options.SharedCredentialsProfile)
		}
		config.Credentials = sharedCredentials
	} else if options.Credentials != nil {
		_, err := options.Credentials.Get()
		if err != nil {
			return nil, errors.Wrap(err, "invalid credentials!")
		}
		config.Credentials = options.Credentials
	}

	sess, err := session.NewSession(config)
	if err != nil {
		return nil, errors.Wrap(err, "problem connecting to AWS")
	}
	svc := s3.New(sess)
	return &s3Bucket{
		name:                options.Name,
		prefix:              options.Prefix,
		compress:            options.Compress,
		singleFileChecksums: options.UseSingleFileChecksums,
		verbose:             options.Verbose,
		sess:                sess,
		svc:                 svc,
		permissions:         options.Permissions,
		contentType:         options.ContentType,
		dryRun:              options.DryRun,
		batchSize:           1000,
		deleteOnPush:        options.DeleteOnPush || options.DeleteOnSync,
		deleteOnPull:        options.DeleteOnPull || options.DeleteOnSync,
	}, nil
}

// NewS3Bucket returns a Bucket implementation backed by S3. This
// implementation does not support multipart uploads, if you would like to add
// objects larger than 5 gigabytes see `NewS3MultiPartBucket`.
func NewS3Bucket(options S3Options) (Bucket, error) {
	bucket, err := newS3BucketBase(nil, options)
	if err != nil {
		return nil, err
	}
	return &s3BucketSmall{s3Bucket: *bucket}, nil
}

// NewS3BucketWithHTTPClient returns a Bucket implementation backed by S3 with
// an existing HTTP client connection. This implementation does not support
// multipart uploads, if you would like to add objects larger than 5
// gigabytes see `NewS3MultiPartBucket`.
func NewS3BucketWithHTTPClient(client *http.Client, options S3Options) (Bucket, error) {
	bucket, err := newS3BucketBase(client, options)
	if err != nil {
		return nil, err
	}
	return &s3BucketSmall{s3Bucket: *bucket}, nil
}

// NewS3MultiPartBucket returns a Bucket implementation backed by S3
// that supports multipart uploads for large objects.
func NewS3MultiPartBucket(options S3Options) (Bucket, error) {
	bucket, err := newS3BucketBase(nil, options)
	if err != nil {
		return nil, err
	}
	// 5MB is the minimum size for a multipart upload, so buffer needs to be at least that big.
	return &s3BucketLarge{s3Bucket: *bucket, minPartSize: 1024 * 1024 * 5}, nil
}

// NewS3MultiPartBucketWithHTTPClient returns a Bucket implementation backed
// by S3 with an existing HTTP client connection that supports multipart
// uploads for large objects.
func NewS3MultiPartBucketWithHTTPClient(client *http.Client, options S3Options) (Bucket, error) {
	bucket, err := newS3BucketBase(client, options)
	if err != nil {
		return nil, err
	}
	// 5MB is the minimum size for a multipart upload, so buffer needs to be at least that big.
	return &s3BucketLarge{s3Bucket: *bucket, minPartSize: 1024 * 1024 * 5}, nil
}

func (s *s3Bucket) String() string { return s.name }

func (s *s3Bucket) Check(ctx context.Context) error {
	input := &s3.HeadBucketInput{
		Bucket: aws.String(s.name),
	}

	_, err := s.svc.HeadBucketWithContext(ctx, input)
	// aside from a 404 Not Found error, HEAD bucket returns a 403
	// Forbidden error. If the latter is the case, that is OK because
	// we know the bucket exists and the given credentials may have
	// access to a sub-bucket. See
	// https://docs.aws.amazon.com/AmazonS3/latest/API/RESTBucketHEAD.html
	// for more information.
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok && aerr.Code() == "NotFound" {
			return errors.Wrap(err, "problem finding bucket")
		}
	}
	return nil
}

type smallWriteCloser struct {
	isClosed    bool
	dryRun      bool
	compress    bool
	verbose     bool
	svc         *s3.S3
	buffer      []byte
	name        string
	ctx         context.Context
	key         string
	permissions S3Permissions
	contentType string
}

type largeWriteCloser struct {
	isCreated      bool
	isClosed       bool
	compress       bool
	dryRun         bool
	verbose        bool
	partNumber     int64
	maxSize        int
	svc            *s3.S3
	ctx            context.Context
	buffer         []byte
	completedParts []*s3.CompletedPart
	name           string
	key            string
	permissions    S3Permissions
	contentType    string
	uploadID       string
}

func (w *largeWriteCloser) create() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer create",
		"bucket":    w.name,
		"key":       w.key,
	})

	if !w.dryRun {
		input := &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(w.name),
			Key:         aws.String(w.key),
			ACL:         aws.String(string(w.permissions)),
			ContentType: aws.String(w.contentType),
		}
		if w.compress {
			input.ContentEncoding = aws.String(compressionEncoding)
		}

		result, err := w.svc.CreateMultipartUploadWithContext(w.ctx, input)
		if err != nil {
			return errors.Wrap(err, "problem creating a multipart upload")
		}
		w.uploadID = *result.UploadId
	}
	w.isCreated = true
	w.partNumber++
	return nil
}

func (w *largeWriteCloser) complete() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer complete",
		"bucket":    w.name,
		"key":       w.key,
	})

	if !w.dryRun {
		input := &s3.CompleteMultipartUploadInput{
			Bucket: aws.String(w.name),
			Key:    aws.String(w.key),
			MultipartUpload: &s3.CompletedMultipartUpload{
				Parts: w.completedParts,
			},
			UploadId: aws.String(w.uploadID),
		}

		_, err := w.svc.CompleteMultipartUploadWithContext(w.ctx, input)
		if err != nil {
			abortErr := w.abort()
			if abortErr != nil {
				return errors.Wrap(abortErr, "problem aborting multipart upload")
			}
			return errors.Wrap(err, "problem completing multipart upload")
		}
	}
	return nil
}

func (w *largeWriteCloser) abort() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer abort",
		"bucket":    w.name,
		"key":       w.key,
	})

	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.name),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	}

	_, err := w.svc.AbortMultipartUploadWithContext(w.ctx, input)
	return err
}

func (w *largeWriteCloser) flush() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer flush",
		"bucket":    w.name,
		"key":       w.key,
	})

	if !w.isCreated {
		err := w.create()
		if err != nil {
			return err
		}
	}
	if !w.dryRun {
		input := &s3.UploadPartInput{
			Body:       aws.ReadSeekCloser(strings.NewReader(string(w.buffer))), // nolint:staticcheck
			Bucket:     aws.String(w.name),
			Key:        aws.String(w.key),
			PartNumber: aws.Int64(w.partNumber),
			UploadId:   aws.String(w.uploadID),
		}
		result, err := w.svc.UploadPartWithContext(w.ctx, input)
		if err != nil {
			abortErr := w.abort()
			if abortErr != nil {
				return errors.Wrap(abortErr, "problem aborting multipart upload")
			}
			return errors.Wrap(err, "problem uploading part")
		}
		w.completedParts = append(w.completedParts, &s3.CompletedPart{
			ETag:       result.ETag,
			PartNumber: aws.Int64(w.partNumber),
		})
	}

	w.buffer = []byte{}
	w.partNumber++
	return nil
}

func (w *smallWriteCloser) Write(p []byte) (int, error) {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "small writer write",
		"bucket":    w.name,
		"key":       w.key,
	})

	if w.isClosed {
		return 0, errors.New("writer already closed")
	}
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *largeWriteCloser) Write(p []byte) (int, error) {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer write",
		"bucket":    w.name,
		"key":       w.key,
	})

	if w.isClosed {
		return 0, errors.New("writer already closed")
	}
	if len(w.buffer)+len(p) > w.maxSize {
		err := w.flush()
		if err != nil {
			return 0, err
		}
	}
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *smallWriteCloser) Close() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "small writer close",
		"bucket":    w.name,
		"key":       w.key,
	})

	if w.isClosed {
		return errors.New("writer already closed")
	}
	if w.dryRun {
		return nil
	}

	input := &s3.PutObjectInput{
		Body:        aws.ReadSeekCloser(strings.NewReader(string(w.buffer))), // nolint:staticcheck
		Bucket:      aws.String(w.name),
		Key:         aws.String(w.key),
		ACL:         aws.String(string(w.permissions)),
		ContentType: aws.String(w.contentType),
	}
	if w.compress {
		input.ContentEncoding = aws.String(compressionEncoding)
	}

	_, err := w.svc.PutObjectWithContext(w.ctx, input)
	return errors.Wrap(err, "problem copying data to file")

}

func (w *largeWriteCloser) Close() error {
	grip.DebugWhen(w.verbose, message.Fields{
		"type":      "s3",
		"dry_run":   w.dryRun,
		"operation": "large writer close",
		"bucket":    w.name,
		"key":       w.key,
	})

	if w.isClosed {
		return errors.New("writer already closed")
	}
	if len(w.buffer) > 0 || w.partNumber == 0 {
		err := w.flush()
		if err != nil {
			return err
		}
	}
	err := w.complete()
	return err
}

type compressingWriteCloser struct {
	gzipWriter io.WriteCloser
	s3Writer   io.WriteCloser
}

func (w *compressingWriteCloser) Write(p []byte) (int, error) {
	return w.gzipWriter.Write(p)
}

func (w *compressingWriteCloser) Close() error {
	catcher := grip.NewBasicCatcher()

	catcher.Add(w.gzipWriter.Close())
	catcher.Add(w.s3Writer.Close())

	return catcher.Resolve()
}

func (s *s3BucketSmall) Writer(ctx context.Context, key string) (io.WriteCloser, error) {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "small writer",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	writer := &smallWriteCloser{
		name:        s.name,
		svc:         s.svc,
		ctx:         ctx,
		key:         s.normalizeKey(key),
		permissions: s.permissions,
		contentType: s.contentType,
		dryRun:      s.dryRun,
		compress:    s.compress,
	}
	if s.compress {
		return &compressingWriteCloser{
			gzipWriter: gzip.NewWriter(writer),
			s3Writer:   writer,
		}, nil
	}
	return writer, nil
}

func (s *s3BucketLarge) Writer(ctx context.Context, key string) (io.WriteCloser, error) {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "large writer",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	writer := &largeWriteCloser{
		maxSize:     s.minPartSize,
		name:        s.name,
		svc:         s.svc,
		ctx:         ctx,
		key:         s.normalizeKey(key),
		permissions: s.permissions,
		contentType: s.contentType,
		dryRun:      s.dryRun,
		compress:    s.compress,
		verbose:     s.verbose,
	}
	if s.compress {
		return &compressingWriteCloser{
			gzipWriter: gzip.NewWriter(writer),
			s3Writer:   writer,
		}, nil
	}
	return writer, nil
}

func (s *s3Bucket) Reader(ctx context.Context, key string) (io.ReadCloser, error) {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "reader",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	input := &s3.GetObjectInput{
		Bucket: aws.String(s.name),
		Key:    aws.String(s.normalizeKey(key)),
	}

	result, err := s.svc.GetObjectWithContext(ctx, input)
	if err != nil {
		if awsErr, ok := err.(awserr.Error); ok && awsErr.Code() == s3.ErrCodeNoSuchKey {
			err = MakeKeyNotFoundError(err)
		}
		return nil, err
	}
	return result.Body, nil
}

func putHelper(ctx context.Context, b Bucket, key string, r io.Reader) error {
	f, err := b.Writer(ctx, key)
	if err != nil {
		return errors.WithStack(err)
	}
	_, err = io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		return errors.Wrap(err, "problem copying data to file")
	}
	return errors.WithStack(f.Close())
}

func (s *s3BucketSmall) Put(ctx context.Context, key string, r io.Reader) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "put",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	return putHelper(ctx, s, key, r)
}

func (s *s3BucketLarge) Put(ctx context.Context, key string, r io.Reader) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "put",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	return putHelper(ctx, s, key, r)
}

func (s *s3Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "get",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	return s.Reader(ctx, key)
}

func (s *s3Bucket) s3WithUploadChecksumHelper(ctx context.Context, target, file string) (bool, error) {
	localmd5, err := md5sum(file)
	if err != nil {
		return false, errors.Wrapf(err, "problem checksumming '%s'", file)
	}
	input := &s3.HeadObjectInput{
		Bucket:  aws.String(s.name),
		Key:     aws.String(target),
		IfMatch: aws.String(localmd5),
	}
	_, err = s.svc.HeadObjectWithContext(ctx, input)
	if aerr, ok := err.(awserr.Error); ok {
		if aerr.Code() == "PreconditionFailed" || aerr.Code() == "NotFound" {
			return true, nil
		}
	}

	return false, errors.Wrapf(err, "problem with checksum for '%s'", target)
}

func doUpload(ctx context.Context, b Bucket, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", path)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, key, f))
}

func (s *s3Bucket) uploadHelper(ctx context.Context, b Bucket, key, path string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "upload",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
		"path":          path,
	})

	if s.singleFileChecksums {
		shouldUpload, err := s.s3WithUploadChecksumHelper(ctx, key, path)
		if err != nil {
			return errors.WithStack(err)
		}
		if !shouldUpload {
			return nil
		}
	}

	return errors.WithStack(doUpload(ctx, b, key, path))
}

func (s *s3BucketLarge) Upload(ctx context.Context, key, path string) error {
	return s.uploadHelper(ctx, s, key, path)
}

func (s *s3BucketSmall) Upload(ctx context.Context, key, path string) error {
	return s.uploadHelper(ctx, s, key, path)
}

func doDownload(ctx context.Context, b Bucket, key, path string) error {
	reader, err := b.Reader(ctx, key)
	if err != nil {
		return errors.WithStack(err)
	}

	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return errors.Wrapf(err, "problem creating enclosing directory for '%s'", path)
	}

	f, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "problem creating file '%s'", path)
	}
	_, err = io.Copy(f, reader)
	if err != nil {
		_ = f.Close()
		return errors.Wrap(err, "problem copying data")
	}

	return errors.WithStack(f.Close())
}

func s3DownloadWithChecksum(ctx context.Context, b Bucket, item BucketItem, local string) error {
	localmd5, err := md5sum(local)
	if os.IsNotExist(errors.Cause(err)) {
		if err = doDownload(ctx, b, item.Name(), local); err != nil {
			return errors.WithStack(err)
		}
	} else if err != nil {
		return errors.WithStack(err)
	}
	if localmd5 != item.Hash() {
		if err = doDownload(ctx, b, item.Name(), local); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (s *s3Bucket) downloadHelper(ctx context.Context, b Bucket, key, path string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "download",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
		"path":          path,
	})

	if s.singleFileChecksums {
		iter, err := s.listHelper(ctx, b, s.normalizeKey(key))
		if err != nil {
			return errors.WithStack(err)
		}
		if !iter.Next(ctx) {
			return errors.New("no results found")
		}
		return s3DownloadWithChecksum(ctx, b, iter.Item(), path)
	}

	return doDownload(ctx, b, key, path)
}

func (s *s3BucketSmall) Download(ctx context.Context, key, path string) error {
	return s.downloadHelper(ctx, s, key, path)
}

func (s *s3BucketLarge) Download(ctx context.Context, key, path string) error {
	return s.downloadHelper(ctx, s, key, path)
}

func (s *s3Bucket) pushHelper(ctx context.Context, b Bucket, opts SyncOptions) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "push",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	files, err := walkLocalTree(ctx, opts.Local)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, fn := range files {
		if re != nil && re.MatchString(fn) {
			continue
		}

		target := consistentJoin(opts.Remote, fn)
		file := filepath.Join(opts.Local, fn)
		shouldUpload, err := s.s3WithUploadChecksumHelper(ctx, target, file)
		if err != nil {
			return errors.WithStack(err)
		}
		if !shouldUpload {
			continue
		}
		if err = doUpload(ctx, b, target, file); err != nil {
			return errors.WithStack(err)
		}
	}

	if s.deleteOnPush && !s.dryRun {
		return errors.Wrap(deleteOnPush(ctx, files, opts.Remote, b), "problem with delete on sync after push")
	}
	return nil
}

func (s *s3BucketSmall) Push(ctx context.Context, opts SyncOptions) error {
	return s.pushHelper(ctx, s, opts)
}
func (s *s3BucketLarge) Push(ctx context.Context, opts SyncOptions) error {
	return s.pushHelper(ctx, s, opts)
}

func (s *s3Bucket) pullHelper(ctx context.Context, b Bucket, opts SyncOptions) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "pull",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	iter, err := b.List(ctx, opts.Remote)
	if err != nil {
		return errors.WithStack(err)
	}

	keys := []string{}
	for iter.Next(ctx) {
		if iter.Err() != nil {
			return errors.Wrap(err, "problem iterating bucket")
		}

		if re != nil && re.MatchString(iter.Item().Name()) {
			continue
		}

		name, err := filepath.Rel(opts.Remote, iter.Item().Name())
		if err != nil {
			return errors.Wrap(err, "problem getting relative filepath")
		}
		localName := filepath.Join(opts.Local, name)
		if err := s3DownloadWithChecksum(ctx, b, iter.Item(), localName); err != nil {
			return errors.WithStack(err)
		}
		keys = append(keys, name)
	}

	if s.deleteOnPull && !s.dryRun {
		return errors.Wrap(deleteOnPull(ctx, keys, opts.Local), "problem with delete on sync after pull")
	}
	return nil
}

func (s *s3BucketSmall) Pull(ctx context.Context, opts SyncOptions) error {
	return s.pullHelper(ctx, s, opts)
}

func (s *s3BucketLarge) Pull(ctx context.Context, opts SyncOptions) error {
	return s.pullHelper(ctx, s, opts)
}

func (s *s3Bucket) Copy(ctx context.Context, options CopyOptions) error {
	if !options.IsDestination {
		options.IsDestination = true
		options.SourceKey = consistentJoin(s.name, s.normalizeKey(options.SourceKey))
		return options.DestinationBucket.Copy(ctx, options)
	}

	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "copy",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"source_key":    options.SourceKey,
		"dest_key":      options.DestinationKey,
	})

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(s.name),
		CopySource: aws.String(options.SourceKey),
		Key:        aws.String(s.normalizeKey(options.DestinationKey)),
		ACL:        aws.String(string(s.permissions)),
	}

	if !s.dryRun {
		_, err := s.svc.CopyObjectWithContext(ctx, input)
		if err != nil {
			return errors.Wrap(err, "problem copying data")
		}
	}
	return nil
}

func (s *s3Bucket) Remove(ctx context.Context, key string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"key":           key,
	})

	if !s.dryRun {
		input := &s3.DeleteObjectInput{
			Bucket: aws.String(s.name),
			Key:    aws.String(s.normalizeKey(key)),
		}

		_, err := s.svc.DeleteObjectWithContext(ctx, input)
		if err != nil {
			return errors.Wrap(err, "problem removing data")
		}
	}
	return nil
}

func (s *s3Bucket) deleteObjectsWrapper(ctx context.Context, toDelete *s3.Delete) error {
	if len(toDelete.Objects) > 0 {
		input := &s3.DeleteObjectsInput{
			Bucket: aws.String(s.name),
			Delete: toDelete,
		}
		_, err := s.svc.DeleteObjectsWithContext(ctx, input)
		if err != nil {
			return errors.Wrap(err, "problem removing data")
		}
	}
	return nil
}

func (s *s3Bucket) RemoveMany(ctx context.Context, keys ...string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"keys":          keys,
	})

	catcher := grip.NewBasicCatcher()
	if !s.dryRun {
		count := 0
		toDelete := &s3.Delete{}
		for _, key := range keys {
			// key limit for s3.DeleteObjectsWithContext, call function and reset
			if count == s.batchSize {
				catcher.Add(s.deleteObjectsWrapper(ctx, toDelete))
				count = 0
				toDelete = &s3.Delete{}
			}
			toDelete.Objects = append(
				toDelete.Objects,
				&s3.ObjectIdentifier{Key: aws.String(s.normalizeKey(key))},
			)
			count++
		}
		catcher.Add(s.deleteObjectsWrapper(ctx, toDelete))
	}
	return catcher.Resolve()
}

func (s *s3BucketSmall) RemovePrefix(ctx context.Context, prefix string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove prefix",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"prefix":        prefix,
	})

	return removePrefix(ctx, prefix, s)
}

func (s *s3BucketLarge) RemovePrefix(ctx context.Context, prefix string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove prefix",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"prefix":        prefix,
	})

	return removePrefix(ctx, prefix, s)
}

func (s *s3BucketSmall) RemoveMatching(ctx context.Context, expression string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove matching",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"expression":    expression,
	})

	return removeMatching(ctx, expression, s)
}

func (s *s3BucketLarge) RemoveMatching(ctx context.Context, expression string) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "remove matching",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"expression":    expression,
	})

	return removeMatching(ctx, expression, s)
}

func (s *s3Bucket) listHelper(ctx context.Context, b Bucket, prefix string) (BucketIterator, error) {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "list",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"prefix":        prefix,
	})

	contents, isTruncated, err := getObjectsWrapper(ctx, s, prefix, "")
	if err != nil {
		return nil, err
	}
	return &s3BucketIterator{
		contents:    contents,
		idx:         -1,
		isTruncated: isTruncated,
		s:           s,
		b:           b,
		prefix:      prefix,
	}, nil
}

func (s *s3BucketSmall) List(ctx context.Context, prefix string) (BucketIterator, error) {
	return s.listHelper(ctx, s, s.normalizeKey(prefix))
}

func (s *s3BucketLarge) List(ctx context.Context, prefix string) (BucketIterator, error) {
	return s.listHelper(ctx, s, s.normalizeKey(prefix))
}

func getObjectsWrapper(ctx context.Context, s *s3Bucket, prefix, marker string) ([]*s3.Object, bool, error) {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(s.name),
		Prefix: aws.String(prefix),
		Marker: aws.String(marker),
	}

	result, err := s.svc.ListObjectsWithContext(ctx, input)
	if err != nil {
		return nil, false, errors.Wrap(err, "problem listing objects")
	}
	return result.Contents, *result.IsTruncated, nil
}

type s3BucketIterator struct {
	contents    []*s3.Object
	idx         int
	isTruncated bool
	err         error
	item        *bucketItemImpl
	s           *s3Bucket
	b           Bucket
	prefix      string
}

func (iter *s3BucketIterator) Err() error { return iter.err }

func (iter *s3BucketIterator) Item() BucketItem { return iter.item }

func (iter *s3BucketIterator) Next(ctx context.Context) bool {
	iter.idx++
	if iter.idx > len(iter.contents)-1 {
		if iter.isTruncated {
			contents, isTruncated, err := getObjectsWrapper(
				ctx,
				iter.s,
				iter.prefix,
				*iter.contents[iter.idx-1].Key,
			)
			if err != nil {
				iter.err = err
				return false
			}
			iter.contents = contents
			iter.idx = 0
			iter.isTruncated = isTruncated
		} else {
			return false
		}
	}

	iter.item = &bucketItemImpl{
		bucket: iter.s.name,
		key:    iter.s.denormalizeKey(*iter.contents[iter.idx].Key),
		hash:   *iter.contents[iter.idx].ETag,
		b:      iter.b,
	}
	return true
}

type s3ArchiveBucket struct {
	*s3BucketLarge
}

// NewS3ArchiveBucket returns a SyncBucket implementation backed by S3 that
// supports syncing the local file system as a single archive file in S3 rather
// than creating an individual object for each file. This SyncBucket is not
// compatible with regular Bucket implementations.
func NewS3ArchiveBucket(options S3Options) (SyncBucket, error) {
	bucket, err := NewS3MultiPartBucket(options)
	if err != nil {
		return nil, err
	}
	return newS3ArchiveBucketWithMultiPart(bucket, options)
}

// NewS3ArchiveBucketWithHTTPClient is the same as NewS3ArchiveBucket but allows
// the user to specify an existing HTTP client connection.
func NewS3ArchiveBucketWithHTTPClient(client *http.Client, options S3Options) (SyncBucket, error) {
	bucket, err := NewS3MultiPartBucketWithHTTPClient(client, options)
	if err != nil {
		return nil, err
	}
	return newS3ArchiveBucketWithMultiPart(bucket, options)
}

func newS3ArchiveBucketWithMultiPart(bucket Bucket, options S3Options) (*s3ArchiveBucket, error) {
	largeBucket, ok := bucket.(*s3BucketLarge)
	if !ok {
		return nil, errors.New("bucket is not a large multipart bucket")
	}
	return &s3ArchiveBucket{s3BucketLarge: largeBucket}, nil
}

const syncArchiveName = "archive.tar"

// Push pushes the contents from opts.Local to the archive prefixed by
// opts.Remote. This operation automatically performs DeleteOnSync in the remote
// regardless of the bucket setting. UseSingleFileChecksums is ignored if it is
// set on the bucket.
func (s *s3ArchiveBucket) Push(ctx context.Context, opts SyncOptions) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"dry_run":       s.dryRun,
		"operation":     "push",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	files, err := walkLocalTree(ctx, opts.Local)
	if err != nil {
		return errors.WithStack(err)
	}

	target := consistentJoin(opts.Remote, syncArchiveName)

	s3Writer, err := s.Writer(ctx, target)
	if err != nil {
		return errors.Wrap(err, "creating writer")
	}
	defer s3Writer.Close()

	tarWriter := tar.NewWriter(s3Writer)
	defer tarWriter.Close()

	for _, fn := range files {
		if re != nil && re.MatchString(fn) {
			continue
		}

		file := filepath.Join(opts.Local, fn)
		// We can't compare the checksum without processing all the local
		// matched files as a tar stream, so just upload it unconditionally.
		if err := tarFile(tarWriter, opts.Local, fn); err != nil {
			return errors.Wrap(err, file)
		}
	}

	return nil
}

// Push pulls the contents from the archive prefixed by opts.Remote to
// opts.Local. UseSingleFileChecksums is ignored if it is set on the bucket.
func (s *s3ArchiveBucket) Pull(ctx context.Context, opts SyncOptions) error {
	grip.DebugWhen(s.verbose, message.Fields{
		"type":          "s3",
		"operation":     "pull",
		"bucket":        s.name,
		"bucket_prefix": s.prefix,
		"remote":        opts.Remote,
		"local":         opts.Local,
		"exclude":       opts.Exclude,
	})

	var re *regexp.Regexp
	var err error
	if opts.Exclude != "" {
		re, err = regexp.Compile(opts.Exclude)
		if err != nil {
			return errors.Wrap(err, "problem compiling exclude regex")
		}
	}

	target := consistentJoin(opts.Remote, syncArchiveName)
	reader, err := s.Get(ctx, target)
	if err != nil {
		return errors.WithStack(err)
	}
	defer reader.Close()

	tarReader := tar.NewReader(reader)
	if err := untar(tarReader, opts.Local, re); err != nil {
		return errors.Wrapf(err, "unarchiving from remote to %s", opts.Local)
	}

	return nil
}
