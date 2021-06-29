package util

import (
	"bytes"

	yaml2 "gopkg.in/yaml.v2"
	"gopkg.in/yaml.v3"
)

// UnmarshalYAMLWithFallback attempts to use yaml v3 to unmarshal, but on failure attempts yaml v2. If this
// succeeds then we can assume in is outdated yaml and requires v2, otherwise we only return the error relevant to the
// current yaml version. This should only be used for cases where we expect v3 to fail for legacy cases.
func UnmarshalYAMLWithFallback(in []byte, out interface{}) error {
	err := yaml.Unmarshal(in, out)
	if err != nil {
		// try the older version of yaml before erroring, in case it's just an oudated yaml
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
			return err
		}
	}
	return nil
}
