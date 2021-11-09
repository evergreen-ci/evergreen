package utility

import (
	"os"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// FileExists provides a clearer interface for checking if a file
// exists.
func FileExists(path string) bool {
	if path == "" {
		return false
	}

	_, err := os.Stat(path)

	return !os.IsNotExist(err)
}

// WriteRawFile writes a sequence of byes to a new file created at the
// specified path.
func WriteRawFile(path string, data []byte) error {
	file, err := os.Create(path)
	if err != nil {
		return errors.Wrapf(err, "problem creating file '%s'", path)
	}

	n, err := file.Write(data)
	if err != nil {
		grip.Warning(message.WrapError(errors.WithStack(file.Close()),
			message.Fields{
				"message":       "problem closing file after error",
				"path":          path,
				"bytes_written": n,
				"input_len":     len(data),
			}))

		return errors.Wrapf(err, "problem writing data to file '%s'", path)
	}

	if err = file.Close(); err != nil {
		return errors.Wrapf(err, "problem closing file '%s' after successfully writing data", path)
	}

	return nil
}

// WriteFile provides a clearer interface for writing string data to a
// file.
func WriteFile(path string, data string) error {
	return errors.WithStack(WriteRawFile(path, []byte(data)))
}
