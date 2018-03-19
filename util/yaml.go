package util

import (
	"encoding/json"
	"fmt"
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

type StringOrBool string

func (v *StringOrBool) MarshalYAML() (interface{}, error) {
	bv, err := v.Bool()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := yaml.Marshal(fmt.Sprintf("%t", bv))

	return out, errors.WithStack(err)
}

func (v *StringOrBool) UnmarshalYAML(um func(interface{}) error) error {
	var val interface{}
	err := um(val)
	if err != nil {
		return errors.WithStack(err)
	}

	return v.setVal(val)
}

func (v *StringOrBool) setVal(val interface{}) error {
	switch input := val.(type) {
	case bool:
		*v = StringOrBool(fmt.Sprintf("%t", input))
	case string:
		*v = StringOrBool(input)
	default:
		return errors.Errorf("'%s' must be a string or a bool [%T]", input, input)
	}

	return nil
}

func (v *StringOrBool) UnmarshalJSON(in []byte) error {
	var val interface{}

	err := json.Unmarshal(in, val)
	if err != nil {
		return errors.WithStack(err)
	}

	return v.setVal(val)
}

func (v *StringOrBool) MarshalJSON() ([]byte, error) {
	bv, err := v.Bool()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := json.Marshal(fmt.Sprintf("%t", bv))
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (v *StringOrBool) Bool() (bool, error) {
	str := string(*v)

	if StringSliceContains([]string{"", "false", "False", "FALSE", "F", "f", "0", "n", "no"}, str) {
		return false, nil
	} else if StringSliceContains([]string{"true", "True", "TRUE", "T", "t", "1", "y", "yes"}, str) {
		return true, nil
	}

	return false, errors.Errorf("'%s' is not a valid string bool form", str)
}
