package task

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// generatedJSONS3Storage implements the GeneratedJSONFileStorage interface to
// access generated JSON files stored in S3.
type generatedJSONS3Storage struct {
	bucket pail.Bucket
	client *http.Client
	closed bool
}

// newGeneratedJSONS3Storage sets up access to generated JSON files stored in
// S3. If this returns a non-nil GeneratedJSONFileStorage, callers are expected
// to call Close when they are finished with it.
func newGeneratedJSONS3Storage(ppConf evergreen.ParserProjectS3Config) (*generatedJSONS3Storage, error) {
	c := utility.GetHTTPClient()

	b, err := pail.NewS3MultiPartBucketWithHTTPClient(c, pail.S3Options{
		Name:   ppConf.Bucket,
		Prefix: ppConf.GeneratedJSONPrefix,
		Region: endpoints.UsEast1RegionID,
	})
	if err != nil {
		utility.PutHTTPClient(c)
		return nil, errors.Wrap(err, "setting up S3 multipart bucket")
	}
	s := generatedJSONS3Storage{
		bucket: b,
		client: c,
	}
	return &s, nil
}

// Find finds the generated JSON files from S3 for the given task.
func (s *generatedJSONS3Storage) Find(ctx context.Context, t *Task) (GeneratedJSONFiles, error) {
	if s.closed {
		return nil, errors.New("cannot access generated JSON file S3 storage when it is closed")
	}

	it, err := s.bucket.List(ctx, t.Id)
	if err != nil {
		return nil, errors.Wrapf(err, "getting generated JSON files for task '%s'", t.Id)
	}

	var files GeneratedJSONFiles
	for it.Next(ctx) {
		item := it.Item()
		file, err := s.downloadFile(ctx, item)
		if err != nil {
			return nil, errors.Wrapf(err, "downloading file for task '%s'", t.Id)
		}
		files = append(files, file)
	}
	if err := it.Err(); err != nil {
		return nil, errors.Wrapf(err, "downloading generated JSON files from S3")
	}

	return files, nil
}

func (s *generatedJSONS3Storage) downloadFile(ctx context.Context, item pail.BucketItem) (string, error) {
	r, err := item.Get(ctx)
	if err != nil {
		return "", errors.Wrapf(err, "downloading generated JSON file '%s'", item.Name())
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return "", errors.Wrapf(err, "reading generated JSON file '%s'", item.Name())
	}

	return string(b), nil
}

// Insert inserts all the generated JSON files for the given task and sets the
// task's generated JSON storage method to S3. If the files are already
// persisted, this will no-op.
func (s *generatedJSONS3Storage) Insert(ctx context.Context, t *Task, files GeneratedJSONFiles) error {
	if s.closed {
		return errors.New("cannot access generated JSON file S3 storage when it is closed")
	}

	if t.GeneratedJSONStorageMethod != "" {
		return nil
	}

	for idx, file := range files {
		r := bytes.NewBufferString(file)
		if err := s.bucket.Put(ctx, s.bucket.Join(t.Id, fmt.Sprint(idx)), r); err != nil {
			return errors.Wrapf(err, "inserting generated JSON file #%d for task '%s'", idx, t.Id)
		}
	}

	if err := t.SetGeneratedJSONStorageMethod(evergreen.ProjectStorageMethodS3); err != nil {
		return errors.Wrapf(err, "settings generated JSON storage method to S3 for task '%s'", t.Id)
	}

	return nil
}

// Close returns the HTTP client that is being used back to the client pool.
func (s *generatedJSONS3Storage) Close(ctx context.Context) error {
	if s.closed {
		return nil
	}

	utility.PutHTTPClient(s.client)
	s.closed = true

	return nil
}
