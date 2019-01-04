package pail

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/pkg/errors"
)

type s3BucketSmall struct {
	s3Bucket
}

type s3BucketLarge struct {
	s3Bucket
	minPartSize int
}

type s3Bucket struct {
	name        string
	prefix      string
	sess        *session.Session
	svc         *s3.S3
	permission  string
	contentType string
	dryRun      bool
}

// S3Options support the use and creation of S3 backed buckets.
type S3Options struct {
	Credentials *credentials.Credentials
	Region      string
	Name        string
	Prefix      string
	Permission  string
	ContentType string
	DryRun      bool
}

// Wrapper for creating AWS credentials.
func CreateAWSCredentials(awsKey, awsPassword, awsToken string) *credentials.Credentials {
	return credentials.NewStaticCredentials(awsKey, awsPassword, awsToken)
}

func (s *s3Bucket) normalizeKey(key string) string {
	if key == "" {
		return s.prefix
	}
	if s.prefix != "" {
		return filepath.Join(s.prefix, key)
	}
	return key
}

func (s *s3Bucket) denormalizeKey(key string) string {
	if s.prefix != "" {
		denormalizedKey, err := filepath.Rel(s.prefix, key)
		if err != nil {
			return key
		}
		return denormalizedKey
	}
	return key
}

func newS3BucketBase(client *http.Client, options S3Options) (*s3Bucket, error) {
	config := &aws.Config{Region: aws.String(options.Region), HTTPClient: client}
	if options.Credentials != nil {
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
		name:        options.Name,
		prefix:      options.Prefix,
		sess:        sess,
		svc:         svc,
		permission:  options.Permission,
		contentType: options.ContentType,
		dryRun:      options.DryRun,
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
	return &s3BucketLarge{s3Bucket: *bucket, minPartSize: 5000000}, nil
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
	return &s3BucketLarge{s3Bucket: *bucket, minPartSize: 5000000}, nil
}

func (s *s3Bucket) String() string { return s.name }

func (s *s3Bucket) Check(ctx context.Context) error {
	input := &s3.GetBucketLocationInput{
		Bucket: aws.String(s.name),
	}

	result, err := s.svc.GetBucketLocationWithContext(ctx, input, s3.WithNormalizeBucketLocation)
	if err != nil {
		return errors.Wrap(err, "problem getting bucket location")
	}
	if *result.LocationConstraint != *s.svc.Client.Config.Region {
		return errors.New("bucket does not exist in given region.")
	}
	return nil
}

type smallWriteCloser struct {
	buffer      []byte
	isClosed    bool
	name        string
	svc         *s3.S3
	ctx         context.Context
	key         string
	permission  string
	contentType string
	dryRun      bool
}

type largeWriteCloser struct {
	buffer         []byte
	maxSize        int
	isCreated      bool
	isClosed       bool
	name           string
	svc            *s3.S3
	ctx            context.Context
	key            string
	permission     string
	contentType    string
	dryRun         bool
	partNumber     int64
	uploadId       string
	completedParts []*s3.CompletedPart
}

func (w *largeWriteCloser) create() error {
	if !w.dryRun {
		input := &s3.CreateMultipartUploadInput{
			Bucket:      aws.String(w.name),
			Key:         aws.String(w.key),
			ACL:         aws.String(w.permission),
			ContentType: aws.String(w.contentType),
		}

		result, err := w.svc.CreateMultipartUploadWithContext(w.ctx, input)
		if err != nil {
			return errors.Wrap(err, "problem creating a multipart upload")
		}
		w.uploadId = *result.UploadId
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
			UploadId: aws.String(w.uploadId),
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
		UploadId: aws.String(w.uploadId),
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
			Body:       aws.ReadSeekCloser(strings.NewReader(string(w.buffer))),
			Bucket:     aws.String(w.name),
			Key:        aws.String(w.key),
			PartNumber: aws.Int64(w.partNumber),
			UploadId:   aws.String(w.uploadId),
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
	w.partNumber++
	return nil
}

func (w *smallWriteCloser) Write(p []byte) (int, error) {
	if w.isClosed {
		return 0, errors.New("writer already closed!")
	}
	w.buffer = append(w.buffer, p...)
	return len(p), nil
}

func (w *largeWriteCloser) Write(p []byte) (int, error) {
	if w.isClosed {
		return 0, errors.New("writer already closed!")
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
		return errors.New("writer already closed!")
	}
	if w.dryRun {
		return nil
	}

	input := &s3.PutObjectInput{
		Body:        aws.ReadSeekCloser(strings.NewReader(string(w.buffer))),
		Bucket:      aws.String(w.name),
		Key:         aws.String(w.key),
		ACL:         aws.String(w.permission),
		ContentType: aws.String(w.contentType),
	}

	_, err := w.svc.PutObjectWithContext(w.ctx, input)
	return errors.Wrap(err, "problem copying data to file")

}

func (w *largeWriteCloser) Close() error {
	if w.isClosed {
		return errors.New("writer already closed!")
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
	return &smallWriteCloser{
		name:        s.name,
		svc:         s.svc,
		ctx:         ctx,
		key:         s.normalizeKey(key),
		permission:  s.permission,
		contentType: s.contentType,
		dryRun:      s.dryRun,
	}, nil
}

func (s *s3BucketLarge) Writer(ctx context.Context, key string) (io.WriteCloser, error) {
	return &largeWriteCloser{
		maxSize:     s.minPartSize,
		name:        s.name,
		svc:         s.svc,
		ctx:         ctx,
		key:         s.normalizeKey(key),
		permission:  s.permission,
		contentType: s.contentType,
		dryRun:      s.dryRun,
	}, nil
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

func putHelper(b Bucket, ctx context.Context, key string, r io.Reader) error {
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
	return putHelper(s, ctx, key, r)
}

func (s *s3BucketLarge) Put(ctx context.Context, key string, r io.Reader) error {
	return putHelper(s, ctx, key, r)
}

func (s *s3Bucket) Get(ctx context.Context, key string) (io.ReadCloser, error) {
	return s.Reader(ctx, key)
}

func uploadHelper(b Bucket, ctx context.Context, key, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", path)
	}
	defer f.Close()

	return errors.WithStack(b.Put(ctx, key, f))
}

func (s *s3BucketSmall) Upload(ctx context.Context, key, path string) error {
	return uploadHelper(s, ctx, key, path)
}

func (s *s3BucketLarge) Upload(ctx context.Context, key, path string) error {
	return uploadHelper(s, ctx, key, path)
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

func (s *s3Bucket) pushHelper(b Bucket, ctx context.Context, local, remote string) error {
	files, err := walkLocalTree(ctx, local)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, fn := range files {
		target := filepath.Join(remote, fn)
		file := filepath.Join(local, fn)
		localmd5, err := md5sum(file)
		if err != nil {
			return errors.Wrapf(err, "problem checksumming '%s'", file)
		}
		input := &s3.HeadObjectInput{
			Bucket:  aws.String(s.name),
			Key:     aws.String(target),
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
	return nil
}

func (s *s3BucketSmall) Push(ctx context.Context, local, remote string) error {
	return s.pushHelper(s, ctx, local, s.normalizeKey(remote))
}

func (s *s3BucketLarge) Push(ctx context.Context, local, remote string) error {
	return s.pushHelper(s, ctx, local, s.normalizeKey(remote))
}

func pullHelper(b Bucket, ctx context.Context, local, remote string) error {
	iter, err := b.List(ctx, remote)
	if err != nil {
		return errors.WithStack(err)
	}

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
	}
	return nil
}

func (s *s3BucketSmall) Pull(ctx context.Context, local, remote string) error {
	return pullHelper(s, ctx, local, remote)
}

func (s *s3BucketLarge) Pull(ctx context.Context, local, remote string) error {
	return pullHelper(s, ctx, local, remote)
}

func (s *s3Bucket) Copy(ctx context.Context, options CopyOptions) error {
	if !options.IsDestination {
		options.IsDestination = true
		options.SourceKey = filepath.Join(s.name, s.normalizeKey(options.SourceKey))
		return options.DestinationBucket.Copy(ctx, options)
	}

	input := &s3.CopyObjectInput{
		Bucket:     aws.String(s.name),
		CopySource: aws.String(options.SourceKey),
		Key:        aws.String(s.normalizeKey(options.DestinationKey)),
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

func (s *s3Bucket) listHelper(b Bucket, ctx context.Context, prefix string) (BucketIterator, error) {
	contents, isTruncated, err := getObjectsWrapper(s, ctx, prefix)
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
	return s.listHelper(s, ctx, s.normalizeKey(prefix))
}

func (s *s3BucketLarge) List(ctx context.Context, prefix string) (BucketIterator, error) {
	return s.listHelper(s, ctx, s.normalizeKey(prefix))
}

func getObjectsWrapper(s *s3Bucket, ctx context.Context, prefix string) ([]*s3.Object, bool, error) {
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
			contents, isTruncated, err := getObjectsWrapper(iter.s, ctx,
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
