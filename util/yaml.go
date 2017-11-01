package util

import (
	"io"
	"io/ioutil"

	"gopkg.in/yaml.v2"
)

// ReadYAMLInto reads data for the given io.ReadCloser - until it hits an error
// or reaches EOF - and attempts to unmarshal the data read into the given
// interface.
func ReadYAMLInto(r io.ReadCloser, data interface{}) error {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bytes, data)
}
