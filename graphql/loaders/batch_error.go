package loaders

import "errors"

type batchError struct {
	err error
}

func (e *batchError) Error() string { return e.err.Error() }
func (e *batchError) Unwrap() error { return e.err }

// IsBatchError reports whether err was produced by a dataloader batch
// operation and has already been logged. The GraphQL error presenter
// should skip logging these to avoid duplicate log entries.
func IsBatchError(err error) bool {
	var be *batchError
	return errors.As(err, &be)
}
