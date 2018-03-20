package util

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
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

func ReadFromYAMLFile(fn string, data interface{}) error {
	if _, err := os.Stat(fn); os.IsNotExist(err) {
		return errors.Errorf("file '%s' does not exist", fn)
	}

	file, err := os.Open(fn)
	if err != nil {
		return errors.Wrapf(err, "problem opening file %s", fn)
	}
	defer file.Close()

	bytes, err := ioutil.ReadAll(file)
	if err != nil {
		return err
	}

	return errors.Wrap(yaml.Unmarshal(bytes, data), "problem reading yaml")
}
