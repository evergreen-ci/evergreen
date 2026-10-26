package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// GeneratedJSONFind is a convenience wrapper to find all generated JSON files
// for the given task from persistent storage.
func GeneratedJSONFind(ctx context.Context, settings *evergreen.Settings, t *Task) (GeneratedJSONFiles, error) {
	fileStorage, err := newGeneratedJSONS3Storage(ctx, settings.Providers.AWS.ParserProject)
	if err != nil {
		return nil, errors.Wrap(err, "getting generated JSON file storage")
	}
	return fileStorage.Find(ctx, t)
}

// GeneratedJSONInsert is a convenience wrapper to insert all generated JSON
// files for the given task into S3.
func GeneratedJSONInsert(ctx context.Context, settings *evergreen.Settings, t *Task, files GeneratedJSONFiles) error {
	fileStorage, err := newGeneratedJSONS3Storage(ctx, settings.Providers.AWS.ParserProject)
	if err != nil {
		return errors.Wrap(err, "getting generated JSON file storage")
	}
	return fileStorage.Insert(ctx, t, files)
}
