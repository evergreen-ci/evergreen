package utility

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// ReadYAML provides an alternate interface to yaml.Unmarshal that
// reads data from an io.ReadCloser.
func ReadYAML(r io.ReadCloser, target interface{}) error {
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(yaml.Unmarshal(data, target))
}

// ReadJSON provides an alternate interface to json.Unmarshal that
// reads data from an io.ReadCloser.
func ReadJSON(r io.ReadCloser, target interface{}) error {
	defer r.Close()
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return errors.WithStack(err)
	}
	return errors.WithStack(json.Unmarshal(data, target))
}

// ReadYAMLFile parses yaml into the target argument from the file
// located at the specifed path.
func ReadYAMLFile(path string, target interface{}) error {
	if !FileExists(path) {
		return errors.Errorf("file '%s' does not exist", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "invalid file: %s", path)
	}

	return errors.Wrapf(ReadYAML(file, target), "problem reading yaml from '%s'", path)
}

// ReadJSONFile parses json into the target argument from the file
// located at the specifed path.
func ReadJSONFile(path string, target interface{}) error {
	if !FileExists(path) {
		return errors.Errorf("file '%s' does not exist", path)
	}

	file, err := os.Open(path)
	if err != nil {
		return errors.Wrapf(err, "invalid file: %s", path)
	}

	return errors.Wrapf(ReadYAML(file, target), "problem reading json from '%s'", path)
}

// PrintJSON marshals the data to a pretty-printed (indented) string
// and then prints it to standard output.
func PrintJSON(data interface{}) error {
	out, err := json.MarshalIndent(data, "", "   ")
	if err != nil {
		return errors.Wrap(err, "problem writing data")
	}

	fmt.Println(string(out))
	return nil
}

// WriteJSONFile marshals the data into json and writes it into a file
// at the specified path.
func WriteJSONFile(fn string, data interface{}) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "problem constructing JSON")
	}

	return errors.WithStack(WriteRawFile(fn, payload))
}

// WriteYAMLFile marshals the data into json and writes it into a file
// at the specified path.
func WriteYAMLFile(fn string, data interface{}) error {
	payload, err := yaml.Marshal(data)
	if err != nil {
		return errors.Wrap(err, "problem constructing YAML")
	}

	return errors.WithStack(WriteRawFile(fn, payload))
}
