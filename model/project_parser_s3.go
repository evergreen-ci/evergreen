package model

import (
	"bytes"
	"context"
	"io/ioutil"
	"net/http"

	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ParserProjectS3Storage implements the ParserProjectStorage interface to
// access parser projects stored in S3.
type ParserProjectS3Storage struct {
	bucket pail.Bucket
	client *http.Client
	closed bool
}

// NewParserProjectS3Storage sets up access to parser projects stored in S3.
// If this returns a non-nil ParserProjectS3Storage, callers are expected to
// call Close when they are finished with it.
func NewParserProjectS3Storage(ppConf evergreen.ParserProjectS3Config) (*ParserProjectS3Storage, error) {
	c := utility.GetHTTPClient()

	var creds *credentials.Credentials
	if ppConf.Key != "" && ppConf.Secret != "" {
		creds = pail.CreateAWSCredentials(ppConf.Key, ppConf.Secret, "")
	}
	b, err := pail.NewS3MultiPartBucketWithHTTPClient(c, pail.S3Options{
		Name:        ppConf.Bucket,
		Prefix:      ppConf.Prefix,
		Region:      endpoints.UsEast1RegionID,
		Credentials: creds,
	})
	if err != nil {
		return nil, errors.Wrap(err, "setting up S3 multipart bucket")
	}
	s := ParserProjectS3Storage{
		bucket: b,
		client: c,
	}
	return &s, nil
}

// FindOneByID finds a parser project in S3 using its ID. If the context errors,
// it will return the context error.
func (s *ParserProjectS3Storage) FindOneByID(ctx context.Context, id string) (*ParserProject, error) {
	if s.closed {
		return nil, errors.New("cannot access parser project S3 storage when it is closed")
	}

	r, err := s.bucket.Get(ctx, id)
	if pail.IsKeyNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "getting parser project '%s'", id)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, errors.Wrapf(err, "reading parser project '%s'", id)
	}

	var pp ParserProject
	if err := bson.Unmarshal(b, &pp); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling parser project '%s' from BSON", id)
	}

	// TODO: this is in the equivalent DB method, but is it necessary?
	if pp.Functions == nil {
		pp.Functions = map[string]*YAMLCommandSet{}
	}

	return &pp, nil
}

// FindOneByIDWithFields finds a parser project using its ID from S3 and returns
// the parser project with only the requested fields populated. This is not any
// more efficient than FindOneByID. If the context errors, it will return the
// context error.
func (s *ParserProjectS3Storage) FindOneByIDWithFields(ctx context.Context, id string, fields ...string) (*ParserProject, error) {
	return s.FindOneByID(ctx, id)
}

// UpsertOne replaces a parser project if the parser project in S3 with the same
// ID already exists. If it does not exist yet, it inserts a new parser project.
func (s *ParserProjectS3Storage) UpsertOne(ctx context.Context, pp *ParserProject) error {
	if s.closed {
		return errors.New("cannot access parser project S3 storage when it is closed")
	}

	b, err := bson.Marshal(pp)
	if err != nil {
		return errors.Wrapf(err, "marshalling parser project '%s' to BSON", pp.Id)
	}

	r := bytes.NewBuffer(b)
	if err := s.bucket.Put(ctx, pp.Id, r); err != nil {
		return errors.Wrapf(err, "upserting parser project '%s'", pp.Id)
	}

	return nil
}

// Close returns the HTTP client that is being used back to the client pool.
func (s *ParserProjectS3Storage) Close(ctx context.Context) error {
	if s.closed {
		return nil
	}

	utility.PutHTTPClient(s.client)
	s.closed = true

	return nil
}
