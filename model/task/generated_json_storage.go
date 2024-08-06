package task

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GeneratedJSONFileStorage is an interface for accessing a task's generated
// JSON for generate.tasks to update the project YAML.
type GeneratedJSONFileStorage interface {
	// FindByTaskID finds all generated JSON files for a given task.
	Find(ctx context.Context, t *Task) (GeneratedJSONFiles, error)
	// Insert inserts all the generated JSON files for the given task. If any
	// of the files already exist, they are replaced.
	Insert(ctx context.Context, t *Task, files GeneratedJSONFiles) error
}

// GetGeneratedJSONFileStorage returns the generated JSON file storage mechanism
// to access the persistent copy of it. Users of the returned
// GeneratedJSONFileStorage must call Close once they are finished using it.
func GetGeneratedJSONFileStorage(settings *evergreen.Settings, method evergreen.ParserProjectStorageMethod) (GeneratedJSONFileStorage, error) {
	switch method {
	case "", evergreen.ProjectStorageMethodDB:
		return generatedJSONDBStorage{}, nil
	case evergreen.ProjectStorageMethodS3:
		return newGeneratedJSONS3Storage(settings.Providers.AWS.ParserProject)
	default:
		return nil, errors.Errorf("unrecognized generated JSON storage method '%s'", method)
	}
}

// GeneratedJSONFind is a convenience wrapper to insert all generated
// JSON files for the given task to persistent storage.
func GeneratedJSONFind(ctx context.Context, settings *evergreen.Settings, t *Task) (GeneratedJSONFiles, error) {
	fileStorage, err := GetGeneratedJSONFileStorage(settings, t.GeneratedJSONStorageMethod)
	if err != nil {
		return nil, errors.Wrap(err, "getting generated JSON file storage")
	}
	return fileStorage.Find(ctx, t)
}

// GeneratedJSONInsert is a convenience wrapper to insert all generated JSON
// files for the given task to persistent storage.
func GeneratedJSONInsert(ctx context.Context, settings *evergreen.Settings, t *Task, files GeneratedJSONFiles, method evergreen.ParserProjectStorageMethod) error {
	fileStorage, err := GetGeneratedJSONFileStorage(settings, method)
	if err != nil {
		return errors.Wrap(err, "getting generated JSON file storage")
	}
	return fileStorage.Insert(ctx, t, files)
}

// GeneratedJSONInsertWithS3Fallback attempts to insert the generated JSON files
// into persistent storage using the given storage method. If it fails due to DB
// document size limitations, it will attempt to fall back to using S3 to store
// it. If it succeeds, this returns the actual storage method used to persist
// the generated JSON files; otherwise, it returns the originally-requested
// storage method.
func GeneratedJSONInsertWithS3Fallback(ctx context.Context, settings *evergreen.Settings, t *Task, files GeneratedJSONFiles, method evergreen.ParserProjectStorageMethod) (evergreen.ParserProjectStorageMethod, error) {
	err := GeneratedJSONInsert(ctx, settings, t, files, method)
	if method == evergreen.ProjectStorageMethodS3 {
		return method, errors.Wrap(err, "inserting generated JSON files into S3")
	}

	if !db.IsDocumentLimit(err) {
		return method, errors.Wrap(err, "inserting generated JSON files into the DB")
	}

	newMethod := evergreen.ProjectStorageMethodS3
	if err := GeneratedJSONInsert(ctx, settings, t, files, newMethod); err != nil {
		return method, errors.Wrap(err, "falling back to generated JSON files into S3")
	}

	grip.Info(message.Fields{
		"message":            "successfully inserted generated JSON files into S3 as fallback due to document size limitation",
		"task_id":            t.Id,
		"old_storage_method": method,
		"new_storage_method": newMethod,
	})

	return newMethod, nil
}
