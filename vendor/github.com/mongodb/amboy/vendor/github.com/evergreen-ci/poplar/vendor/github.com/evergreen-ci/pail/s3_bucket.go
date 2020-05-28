package pail

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	homedir "github.com/mitchellh/go-homedir"
	"github.com/mongodb/grip"
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
	dryRun       bool
	deleteOnSync bool
	compress     bool
	batchSize    int
	sess         *session.Session
	svc          *s3.S3
	name         string
	prefix       string
	permissions  S3Permissions
	contentType  string
}

// S3Options support the use and creation of S3 backed buckets.
type S3Options struct {
	DryRun                    bool
	DeleteOnSync              bool
	Compress                  bool
	MaxRetries                int
	Credentials               *credentials.Credentials
	SharedCredentialsFilepath string
	SharedCredentialsProfile  string
	Region                    string
	Name                      string
	Prefix                    string
	Permissions               S3Permissions
	ContentType               string
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

	config := &aws.Config{
		Region:     aws.String(options.Region),
		HTTPClient: client,
		MaxRetries: aws.Int(options.MaxRetries),
	}
	// if options.SharedCredentialsProfile is set, will override any credentials passed in
	if options.SharedCredentialsProfile != "" {
		fp := options.SharedCredentialsFilepath
		if fp == "" {
			// if options.SharedCredentialsFilepath is not set, use default filepath
			var homeDir string
			var err error
			if runtime.GOOS == "windows" {
				homeDir = os.Getenv("USERPROFILE")
			} else {
				homeDir, err = homedir.Dir()
			}
			if err != nil {
				return nil, errors.Wrap(err, "failed to detect home directory when getting default credentials file")
			}
			fp = filepath.Join(homeDir, ".aws", "credentials")
		}
		sharedCredentials := credentials.NewSharedCredentials(fp, options.SharedCredentialsProfile)
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
		name:         options.Name,
		prefix:       options.Prefix,
		compress:     options.Compress,
		sess:         sess,
		svc:          svc,
		permissions:  options.Permissions,
		contentType:  options.ContentType,
		dryRun:       options.DryRun,
		batchSize:    1000,
		deleteOnSync: options.DeleteOnSync,
	}, nil
}

// NewS3Bucket returns a Bucket implementation backed by S3. This
// implementation does not support multipart uploads, if you would
// like to add objects larger than 5 gigabytes see
// `NewS3MultiPartBucket`.
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
	input := &s3.AbortMultipartUploadInput{
		Bucket:   aws.String(w.name),
		Key:      aws.String(w.key),
		UploadId: aws.String(w.uploadID),
	}

	_, err := w.svc.AbortMultipartUploadWithContext(w.ctx, input)
	return err
}

func (w *largeWriteCloser) flush() error {
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
	if w.isClosed {
		return 0, errors.New("writer already closed")
	}
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *largeWriteCloser) Write(p []byte) (int, error) {
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

func (w *largeWriteCloser) Close() error {
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

func (s *s3BucketSmall) Writer(ctx context.Context, key string) (io.WriteCloser, error) {
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
	input := &s3.GetObjectInput{
		Bucket: aws.String(s.name),
		Key:    aws.String(s.normalizeKey(key)),
	}

	result, err := s.svc.GetObjectWithContext(ctx, input)
	if err != nil {
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
	return putHelper(ctx, s, key, r)
}

func (s *s3BucketLarge) Put(ctx context.Context, key string, r io.Reader) error {
	return putHelper(ctx, s, key, r)
}

func (s *s3Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.Reader(ctx, key)
}

func uploadHelper(ctx context.Context, b Bucket, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", path)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, key, f))
}

func (s *s3BucketSmall) Upload(ctx context.Context, key, path string) error {
	return uploadHelper(ctx, s, key, path)
}

func (s *s3BucketLarge) Upload(ctx context.Context, key, path string) error {
	return uploadHelper(ctx, s, key, path)
}

func (s *s3Bucket) Download(ctx context.Context, key, path string) error {
	reader, err := s.Reader(ctx, key)
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

func (s *s3Bucket) push(ctx context.Context, local, remote string, b Bucket) error {
	files, err := walkLocalTree(ctx, local)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, fn := range files {
		target := consistentJoin(remote, fn)
		file := filepath.Join(local, fn)
		localmd5, err := md5sum(file)
		if err != nil {
			return errors.Wrapf(err, "problem checksumming '%s'", file)
		}
		input := &s3.HeadObjectInput{
			Bucket:  aws.String(s.name),
			Key:     aws.String(s.normalizeKey(target)),
			IfMatch: aws.String(localmd5),
		}
		_, err = s.svc.HeadObjectWithContext(ctx, input)
		if aerr, ok := err.(awserr.Error); ok {
			if aerr.Code() == "PreconditionFailed" || aerr.Code() == "NotFound" {
				if err = b.Upload(ctx, target, file); err != nil {
					return errors.Wrapf(err, "problem uploading '%s' to '%s'",
						file, target)
				}
			}
		} else if err != nil {
			return errors.Wrapf(err, "problem finding '%s'", target)
		}
	}

	if s.deleteOnSync && !s.dryRun {
		return errors.Wrapf(os.RemoveAll(local), "problem removing '%s' after push", local)
	}
	return nil
}

func (s *s3BucketSmall) Push(ctx context.Context, local, remote string) error {
	return s.push(ctx, local, remote, s)
}

func (s *s3BucketLarge) Push(ctx context.Context, local, remote string) error {
	return s.push(ctx, local, remote, s)
}

func (s *s3Bucket) pull(ctx context.Context, local, remote string, b Bucket) error {
	iter, err := b.List(ctx, remote)
	if err != nil {
		return errors.WithStack(err)
	}

	keys := []string{}
	for iter.Next(ctx) {
		if iter.Err() != nil {
			return errors.Wrap(err, "problem iterating bucket")
		}
		name, err := filepath.Rel(remote, iter.Item().Name())
		if err != nil {
			return errors.Wrap(err, "problem getting relative filepath")
		}
		localName := filepath.Join(local, name)
		localmd5, err := md5sum(localName)
		if os.IsNotExist(errors.Cause(err)) {
			if err = b.Download(ctx, iter.Item().Name(), localName); err != nil {
				return errors.WithStack(err)
			}
		} else if err != nil {
			return errors.WithStack(err)
		}
		if localmd5 != iter.Item().Hash() {
			if err = b.Download(ctx, iter.Item().Name(), localName); err != nil {
				return errors.WithStack(err)
			}
		}

		keys = append(keys, iter.Item().Name())
	}

	if s.deleteOnSync && !s.dryRun {
		return errors.Wrapf(b.RemoveMany(ctx, keys...), "problem removing '%s' after pull", remote)
	}
	return nil
}

func (s *s3BucketSmall) Pull(ctx context.Context, local, remote string) error {
	return s.pull(ctx, local, remote, s)
}

func (s *s3BucketLarge) Pull(ctx context.Context, local, remote string) error {
	return s.pull(ctx, local, remote, s)
}

func (s *s3Bucket) Copy(ctx context.Context, options CopyOptions) error {
	if !options.IsDestination {
		options.IsDestination = true
		options.SourceKey = consistentJoin(s.name, s.normalizeKey(options.SourceKey))
		return options.DestinationBucket.Copy(ctx, options)
	}

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
	return removePrefix(ctx, prefix, s)
}

func (s *s3BucketLarge) RemovePrefix(ctx context.Context, prefix string) error {
	return removePrefix(ctx, prefix, s)
}

func (s *s3BucketSmall) RemoveMatching(ctx context.Context, expression string) error {
	return removeMatching(ctx, expression, s)
}

func (s *s3BucketLarge) RemoveMatching(ctx context.Context, expression string) error {
	return removeMatching(ctx, expression, s)
}

func (s *s3Bucket) listHelper(ctx context.Context, b Bucket, prefix string) (BucketIterator, error) {
	contents, isTruncated, err := getObjectsWrapper(ctx, s, prefix)
	if err != nil {
		return nil, err
	}
	return &s3BucketIterator{
		contents:    contents,
		idx:         -1,
		isTruncated: isTruncated,
		s:           s,
		b:           b,
	}, nil
}

func (s *s3BucketSmall) List(ctx context.Context, prefix string) (BucketIterator, error) {
	return s.listHelper(ctx, s, s.normalizeKey(prefix))
}

func (s *s3BucketLarge) List(ctx context.Context, prefix string) (BucketIterator, error) {
	return s.listHelper(ctx, s, s.normalizeKey(prefix))
}

func getObjectsWrapper(ctx context.Context, s *s3Bucket, prefix string) ([]*s3.Object, bool, error) {
	input := &s3.ListObjectsInput{
		Bucket: aws.String(s.name),
		Prefix: aws.String(prefix),
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
}

func (iter *s3BucketIterator) Err() error { return iter.err }

func (iter *s3BucketIterator) Item() BucketItem { return iter.item }

func (iter *s3BucketIterator) Next(ctx context.Context) bool {
	iter.idx++
	if iter.idx > len(iter.contents)-1 {
		if iter.isTruncated {
			contents, isTruncated, err := getObjectsWrapper(ctx, iter.s,
				*iter.contents[iter.idx-1].Key)
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
