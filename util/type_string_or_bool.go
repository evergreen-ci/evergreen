package util

import (
	"encoding/json"
	"fmt"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

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
	var val string
	err := um(&val)
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
	var val string

	err := json.Unmarshal(in, &val)
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
	if v == nil {
		return false, nil
	}

	str := string(*v)

	if StringSliceContains([]string{"", "false", "False", "FALSE", "F", "f", "0", "n", "no", "!true", "!t", "!True"}, str) {
		return false, nil
	} else if StringSliceContains([]string{"true", "True", "TRUE", "T", "t", "1", "y", "yes", "!false", "!f", "!False"}, str) {
		return true, nil
	}

	return false, errors.Errorf("'%s' is not a valid string bool form", str)
}
