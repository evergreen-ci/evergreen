package util

import (
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

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
