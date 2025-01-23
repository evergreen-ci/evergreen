package task

import (
	"context"
)

// generatedJSONDBStorage implements the generatedJSONDBStorage interface to
// access generated JSON files stored in a task document in the DB.
type generatedJSONDBStorage struct {
}

// Find finds the generated JSON from the DB for the given task. This ignores
// the context parameter.
func (s generatedJSONDBStorage) Find(_ context.Context, t *Task) (GeneratedJSONFiles, error) {
	return t.GeneratedJSONAsString, nil
}

// Insert inserts all the generated JSON files for the given task. If the files
// are already persisted, this will no-op.
func (s generatedJSONDBStorage) Insert(ctx context.Context, t *Task, files GeneratedJSONFiles) error {
	return t.SetGeneratedJSON(ctx, files)
}
