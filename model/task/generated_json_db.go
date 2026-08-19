package task

import (
	"context"
)

// generatedJSONDBStorage implements the generatedJSONDBStorage interface to
// access generated JSON files stored in a task document in the DB.
//
// TODO (DEVPROD-41456): delete this type. All new generated JSON is written to
// S3; this only remains to read tasks written before that switch.
type generatedJSONDBStorage struct {
}

// Find finds the generated JSON from the DB for the given task. This ignores
// the context parameter.
func (s generatedJSONDBStorage) Find(_ context.Context, t *Task) (GeneratedJSONFiles, error) {
	return t.GeneratedJSONAsString, nil
}
