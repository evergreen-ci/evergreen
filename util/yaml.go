package util

import (
	"bytes"

	"github.com/pkg/errors"
	yaml "gopkg.in/20210107192922/yaml.v3"
	yaml2 "gopkg.in/yaml.v2"
	upgradedYAML "gopkg.in/yaml.v3"
)

const UnmarshalStrictError = "error unmarshalling yaml strict"

// UnmarshalYAMLWithFallback attempts to use yaml v3 to unmarshal, but on failure attempts yaml v2. If this
// succeeds then we can assume in is outdated yaml and requires v2, otherwise we only return the error relevant to the
// current yaml version. This should only be used for cases where we expect v3 to fail for legacy cases.
func UnmarshalYAMLWithFallback(in []byte, out interface{}) error {
	err := yaml.Unmarshal(in, out)
	if err != nil {
		// try the older version of yaml before erroring, in case it's just an outdated yaml
		if err2 := yaml2.Unmarshal(in, out); err2 != nil {
			return err
		}
	}
	return nil
}

// UnmarshalYAMLStrictWithFallback attempts to use yaml v3 to unmarshal strict, but on failure attempts yaml v2. If this
// succeeds then we can assume in is outdated yaml and requires v2, otherwise we only return the error relevant to the
// current yaml version. This should only be used for cases where we expect v3 to fail for legacy cases.
func UnmarshalYAMLStrictWithFallback(in []byte, out interface{}) error {
	d := yaml.NewDecoder(bytes.NewReader(in))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		if err2 := yaml2.UnmarshalStrict(in, out); err2 != nil {
			return errors.Wrap(err, UnmarshalStrictError)
		}
	}
	return nil
}

// UnmarshalUpgradedYAMLWithFallback is identical to UnmarshalYAMLWithFallback but uses the upgraded YAML version
// (v3.0.1) instead of the current YAML version.
// This should only be used for user validation to see if their project's YAML hypothetically produces a valid config
// should the YAML version be upgraded.
func UnmarshalUpgradedYAMLWithFallback(in []byte, out interface{}) error {
	if err := upgradedYAML.Unmarshal(in, out); err != nil {
		// try the older version of yaml before erroring, in case it's just an outdated yaml
		if err2 := yaml2.Unmarshal(in, out); err2 != nil {
			return err
		}
	}
	return nil
}

// UnmarshalUpgradedYAMLStrictWithFallback is identical to UnmarshalYAMLStrictWithFallback but uses the upgraded YAML
// version (v3.0.1) instead of the current YAML version.
// This should only be used for user validation to see if their project's YAML hypothetically produces a valid config
// should the YAML version be upgraded.
func UnmarshalUpgradedYAMLStrictWithFallback(in []byte, out interface{}) error {
	d := upgradedYAML.NewDecoder(bytes.NewReader(in))
	d.KnownFields(true)
	if err := d.Decode(out); err != nil {
		// try the older version of yaml before erroring, in case it's just an outdated yaml
		if err2 := yaml2.Unmarshal(in, out); err2 != nil {
			return errors.Wrap(err, UnmarshalStrictError)
		}
	}
	return nil
}
