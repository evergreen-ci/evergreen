package poplar

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"strings"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

// LoadReport reads the content of the specified file and attempts to create a
// Report structure based on the content. The file can be in bson, json, or
// yaml, and LoadReport examines the file's extension to determine the data
// format. If the bucket API key, secret, or token are not populated, the
// corresponding environment variables will be used to populate the values.
func LoadReport(fn string) (*Report, error) {
	out := &Report{}
	if err := readFile(fn, out); err != nil {
		return nil, err
	}

	if out.BucketConf.APIKey == "" {
		out.BucketConf.APIKey = os.Getenv(APIKeyEnv)
	}
	if out.BucketConf.APISecret == "" {
		out.BucketConf.APISecret = os.Getenv(APISecretEnv)
	}
	if out.BucketConf.APIToken == "" {
		out.BucketConf.APIToken = os.Getenv(APITokenEnv)
	}

	return out, nil
}

// LoadTests reads the content of the specified file and attempts to create a
// Report structure based on the content. The file can be in json or yaml and
// LoadTests examines the file's extension to determine the data format. Note
// that this expects a subset of the actual Report data, as an array of Test
// structures, and will return a Report instance with empty fields except for
// the `Tests` field.
func LoadTests(fn string) (*Report, error) {
	if strings.HasSuffix(fn, ".bson") {
		return nil, errors.New("cannot load an array of tests from bson")
	}

	out := []Test{}
	if err := readFile(fn, &out); err != nil {
		return nil, err
	}

	report := &Report{}
	report.Tests = out
	return report, nil
}

func readFile(fn string, out interface{}) error {
	if stat, err := os.Stat(fn); os.IsNotExist(err) || stat.IsDir() {
		return errors.Errorf("'%s' does not exist", fn)
	}

	unmarshal := getUnmarshaler(fn)
	if unmarshal == nil {
		return errors.Errorf("cannot find unmarshler for input %s", fn)
	}

	data, err := ioutil.ReadFile(fn)
	if err != nil {
		return errors.Wrapf(err, "problem reading data from %s", fn)
	}

	return errors.Wrap(unmarshal(data, out), "problem unmarshaling report data")
}
