package util

import (
	"encoding/json"
	"io"
	"io/ioutil"
)

// ReadJSONInto reads JSON from an io.ReadCloser into the data pointer.
func ReadJSONInto(r io.ReadCloser, data interface{}) error {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, data)
}
