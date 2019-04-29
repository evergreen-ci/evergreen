package registry

import (
	"encoding/json"

	"github.com/mongodb/amboy"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
	yaml "gopkg.in/yaml.v2"
)

// ConvertTo takes a Format specification and interface and returns a
// serialized byte sequence according to that Format value. If there
// is an issue with the serialization, or the Format value is not
// supported, then this method returns an error.
func convertTo(f amboy.Format, v interface{}) ([]byte, error) {
	var output []byte
	var err error

	switch f {
	case amboy.JSON:
		output, err = json.Marshal(v)
	case amboy.YAML:
		output, err = yaml.Marshal(v)
	case amboy.BSON:
		output, err = mgobson.Marshal(v)
	case amboy.BSON2:
		output, err = bson.Marshal(v)
	default:
		return nil, errors.New("no support for specified serialization format")
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem serializing data")
	}

	return output, nil

}

// ConvertFrom takes a Format type, a byte sequence, and an interface
// and attempts to deserialize that data into the interface object as
// indicated by the Format specifier.
func convertFrom(f amboy.Format, data []byte, v interface{}) error {
	switch f {
	case amboy.JSON:
		return errors.Wrap(json.Unmarshal(data, v), "problem serializing data from json")
	case amboy.BSON:
		return errors.Wrap(mgobson.Unmarshal(data, v), "problem serializing data from bson")
	case amboy.BSON2:
		return errors.Wrap(bson.Unmarshal(data, v), "problem serializing data from bson (new)")
	case amboy.YAML:
		return errors.Wrap(yaml.Unmarshal(data, v), "problem serializing data from yaml")
	default:
		return errors.New("no support for specified serialization format")
	}
}
