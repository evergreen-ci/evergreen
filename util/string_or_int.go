package util

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

type StringOrInt string

func (v *StringOrInt) MarshalYAML() (interface{}, error) {
	bv, err := v.Int()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := yaml.Marshal(fmt.Sprintf("%d", bv))
	return out, errors.WithStack(err)
}

func (v *StringOrInt) UnmarshalYAML(um func(interface{}) error) error {
	var val interface{}
	err := um(val)
	if err != nil {
		return errors.WithStack(err)
	}

	return v.setVal(val)
}

func (v *StringOrInt) setVal(val interface{}) error {
	switch input := val.(type) {
	case int:
		*v = StringOrInt(fmt.Sprintf("%d", input))
	case string:
		*v = StringOrInt(input)
	default:
		return errors.Errorf("'%s' must be a string or a int [%T]", input, input)
	}

	return nil
}

func (v *StringOrInt) UnmarshalJSON(in []byte) error {
	var val interface{}

	err := json.Unmarshal(in, val)
	if err != nil {
		return errors.WithStack(err)
	}

	return v.setVal(val)
}

func (v *StringOrInt) MarshalJSON() ([]byte, error) {
	bv, err := v.Int()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := json.Marshal(fmt.Sprintf("%d", bv))
	if err != nil {
		return nil, err
	}

	return out, nil
}

func (v *StringOrInt) Int() (int, error) {
	if v == nil {
		return 0, nil
	}

	str := string(*v)
	i, err := strconv.ParseInt(str, 10, 64)
	if err != nil {
		return 0, errors.Wrapf(err, "'%s' cannot be parsed as an int", str)
	}
	return int(i), nil
}
