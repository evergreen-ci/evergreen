package util

import (
	"bytes"

	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

const UnmarshalStrictError = "error unmarshalling yaml strict"

// UnmarshalYAML unmarshals the input YAML using yaml.v3.
func UnmarshalYAML(in []byte, out any) error {
	return yaml.Unmarshal(in, out)
}

// UnmarshalYAMLStrict unmarshals the input YAML using yaml.v3 in strict mode, erroring on unknown fields.
func UnmarshalYAMLStrict(in []byte, out any) error {
	d := yaml.NewDecoder(bytes.NewReader(in))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		return errors.Wrap(err, UnmarshalStrictError)
	}
	return nil
}
