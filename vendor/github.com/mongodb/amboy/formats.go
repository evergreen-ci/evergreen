package amboy

import (
	"encoding/json"

	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

// Format defines a sequence of constants used to distinguish between
// different serialization formats for job objects used in the
// amboy.ConvertTo and amboy.ConvertFrom functions, which support the
// functionality of the Export and Import methods in the job
// interface.
type Format int

// Supported values of the Format type, which represent different
// supported serialization methods..
const (
	BSON Format = iota
	YAML
	JSON
)

// ConvertTo takes a Format specification and interface and returns a
// serialized byte sequence according to that Format value. If there
// is an issue with the serialization, or the Format value is not
// supported, then this method returns an error.
func ConvertTo(f Format, v interface{}) ([]byte, error) {
	var output []byte
	var err error

	switch f {
	case JSON:
		output, err = json.Marshal(v)
	case BSON:
		output, err = bson.Marshal(v)
	case YAML:
		output, err = yaml.Marshal(v)
	default:
		return nil, errors.New("no support for specified serialization format")
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem serializing data")
	}

	return output, nil

}

// ConvertFrom takes a Format type, a byte sequence, and an interface
// and attempts to serialize that data into the interface object as
// indicated by the Format specifier.
func ConvertFrom(f Format, data []byte, v interface{}) error {
	switch f {
	case JSON:
		return errors.Wrap(json.Unmarshal(data, v), "problem serializing data from json")
	case BSON:
		return errors.Wrap(bson.Unmarshal(data, v), "problem serializing data from bson")
	case YAML:
		return errors.Wrap(yaml.Unmarshal(data, v), "problem serializing data from yaml")
	default:
		return errors.New("no support for specified serialization format")
	}
}
