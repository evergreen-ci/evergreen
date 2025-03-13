package model

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/pail"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ParserProjectS3Storage implements the ParserProjectStorage interface to
// access parser projects stored in S3.
type ParserProjectS3Storage struct {
	bucket pail.Bucket
}

// NewParserProjectS3Storage sets up access to parser projects stored in S3.
func NewParserProjectS3Storage(ctx context.Context, ppConf evergreen.ParserProjectS3Config) (*ParserProjectS3Storage, error) {
	b, err := pail.NewS3MultiPartBucket(ctx, pail.S3Options{
		Name:   ppConf.Bucket,
		Prefix: ppConf.Prefix,
		Region: evergreen.DefaultEC2Region,
	})
	if err != nil {
		return nil, errors.Wrap(err, "setting up S3 multipart bucket")
	}
	return &ParserProjectS3Storage{bucket: b}, nil
}

// FindOneByID finds a parser project in S3 using its ID. If the context errors,
// it will return the context error.
func (s *ParserProjectS3Storage) FindOneByID(ctx context.Context, id string) (*ParserProject, error) {
	r, err := s.bucket.Get(ctx, id)
	if pail.IsKeyNotFoundError(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "getting parser project '%s'", id)
	}
	defer r.Close()

	b, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrapf(err, "reading parser project '%s'", id)
	}

	var pp ParserProject
	if err := bson.Unmarshal(b, &pp); err != nil {
		return nil, errors.Wrapf(err, "unmarshalling parser project '%s' from BSON", id)
	}

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
	bsonPP, err := bson.Marshal(pp)
	if err != nil {
		return errors.Wrapf(err, "marshalling parser project '%s' to BSON", pp.Id)
	}
	parserProjectLen := len(bsonPP)
	config, err := evergreen.GetConfig(ctx)
	if err != nil {
		return errors.Wrap(err, "getting config")
	}
	isDegradedMode := !config.ServiceFlags.CPUDegradedModeDisabled
	maxSize := config.TaskLimits.MaxParserProjectSize
	if isDegradedMode {
		maxSize = config.TaskLimits.MaxDegradedModeParserProjectSize
	}
	// Multiply 1024*1024 to get the full size in MB
	if maxSize > 0 && parserProjectLen > maxSize*1024*1024 {
		serviceMode := "standard"
		if isDegradedMode {
			serviceMode = "degraded"
		}
		parserProjectMB := float64(parserProjectLen) / (1024 * 1024)
		errMsg := fmt.Sprintf("parser project exceeds the %s system limit (%f MB > %v MB).", serviceMode, parserProjectMB, maxSize)
		return errors.New(errMsg)
	}
	return s.UpsertOneBSON(ctx, pp.Id, bsonPP)
}

// UpsertOneBSON upserts a parser project by its ID when has already been
// marshalled to BSON.
func (s *ParserProjectS3Storage) UpsertOneBSON(ctx context.Context, id string, bsonPP []byte) error {
	r := bytes.NewBuffer(bsonPP)
	if err := s.bucket.Put(ctx, id, r); err != nil {
		return errors.Wrapf(err, "upserting BSON parser project '%s'", id)
	}

	return nil
}
