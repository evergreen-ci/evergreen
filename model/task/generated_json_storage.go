package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/pkg/errors"
)

// GeneratedJSONFileStorage is an interface for accessing a task's generated
// JSON for generate.tasks to update the project YAML.
type GeneratedJSONFileStorage interface {
	// FindByTaskID finds all generated JSON files for a given task.
	Find(ctx context.Context, t *Task) (GeneratedJSONFiles, error)
}

// GetGeneratedJSONFileStorage returns the generated JSON file storage mechanism
// to access the persistent copy of it. Users of the returned
// GeneratedJSONFileStorage must call Close once they are finished using it.
func GetGeneratedJSONFileStorage(ctx context.Context, settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod) (GeneratedJSONFileStorage, error) {
	switch method {
	case "", evergreen.ProjectStorageMethodDB:
		return generatedJSONDBStorage{}, nil
	case evergreen.ProjectStorageMethodS3:
		return newGeneratedJSONS3Storage(ctx, settings.Providers.AWS.ParserProject)
	default:
		return nil, errors.Errorf("unrecognized generated JSON storage method '%s'", method)
	}
}

// GeneratedJSONFind is a convenience wrapper to insert all generated
// JSON files for the given task to persistent storage.
func GeneratedJSONFind(ctx context.Context, settings *evergreen.Settings, t *Task) (GeneratedJSONFiles, error) {
	fileStorage, err := GetGeneratedJSONFileStorage(ctx, settings, t.GeneratedJSONStorageMethod)
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
