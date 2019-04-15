package util

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/pkg/errors"
)

// ReadJSONInto reads JSON from an io.ReadCloser into the data pointer.
func ReadJSONInto(r io.ReadCloser, data interface{}) error {
	_, err := ReadJSONIntoWithLength(r, data)
	return err
}

// ReadJSONInto reads JSON from an io.ReadCloser into the data pointer.
func ReadJSONIntoWithLength(r io.ReadCloser, data interface{}) (int, error) {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return 0, errors.Wrapf(err, "error reading JSON (%s)", string(bytes))
	}
	length := len(bytes)

	return length, errors.Wrapf(json.Unmarshal(bytes, data), "error attempting to unmarshal into %T", data)
}

func WriteJSONInto(fn string, data interface{}) error {
	out, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "problem writing JSON")
	}

	return errors.Wrap(ioutil.WriteFile(fn, out, 0667), "problem writing data")
}
