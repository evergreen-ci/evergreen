package util

import (
	"encoding/json"
	"io"
	"io/ioutil"

	"github.com/pkg/errors"
)

// ReadJSONInto reads JSON from an io.ReadCloser into the data pointer.
func ReadJSONInto(r io.ReadCloser, data interface{}) error {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.Wrapf(err, "error reading JSON (%s)", string(bytes))
	}
	return errors.Wrapf(json.Unmarshal(bytes, data), "error attempting to unmarshal into %T", data)
}

func WriteJSONInto(fn string, data interface{}) error {
	out, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "problem writing JSON")
	}

	return errors.Wrap(ioutil.WriteFile(fn, out, 0667), "problem writing data")
}
