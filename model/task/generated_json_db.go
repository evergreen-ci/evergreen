package task

import (
	"context"
)

// GeneratedJSONDBStorage implements the GeneratedJSONDBStorage interface to
// access generated JSON files stored in the DB.
type GeneratedJSONDBStorage struct {
}

// Find finds the generated JSON from the DB for the given task. This ignores
// the context parameter.
func (s GeneratedJSONDBStorage) Find(_ context.Context, t *Task) (GeneratedJSONFiles, error) {
	return t.GeneratedJSONAsString, nil
}

// Insert inserts all the generated JSON files for the given task. If the files
// are already persisted, this will no-op.
func (s GeneratedJSONDBStorage) Insert(_ context.Context, t *Task, files GeneratedJSONFiles) error {
	return t.SetGeneratedJSON(files)
}

// Close is a no-op.
func (s GeneratedJSONDBStorage) Close(context.Context) error {
	return nil
}
