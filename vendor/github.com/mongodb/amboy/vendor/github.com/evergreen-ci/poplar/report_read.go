package poplar

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/mongodb/mongo-go-driver/bson"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

type unmarshaler func([]byte, interface{}) error

func getUnmarshaler(fn string) unmarshaler {
	switch {
	case strings.HasSuffix(fn, ".bson"):
		return bson.Unmarshal
	case strings.HasSuffix(fn, ".json"):
		return json.Unmarshal
	case strings.HasSuffix(fn, ".yaml"), strings.HasSuffix(fn, ".yml"):
		return yaml.Unmarshal
	default:
		return nil
	}
}

// LoadReport reads the content of the specified file and attempts to
// create a Report structure based on the content. The file can be in
// bson, json, or yaml, and LoadReport examines the files' extension
// to determine the data format.
func LoadReport(fn string) (*Report, error) {
	if stat, err := os.Stat(fn); os.IsNotExist(err) || stat.IsDir() {
		return nil, errors.Errorf("'%s' does not exist", fn)
	}

	unmarshal := getUnmarshaler(fn)
	if unmarshal == nil {
		return nil, errors.Errorf("cannot find unmarshler for input %s", fn)
	}

	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return nil, errors.Wrapf(err, "problem reading data from %s", fn)
	}

	out := Report{}
	if err = unmarshal(data, &out); err != nil {
		return nil, errors.Wrap(err, "problem unmarshaling report data")
	}

	return &out, nil
}
