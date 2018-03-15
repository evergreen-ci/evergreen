package util

import (
	"bytes"
	"encoding/gob"

	"github.com/pkg/errors"
)

// DeepCopy makes a deep copy of the src value into the copy params
// The registeredTypes param can be optionally used to register additional types
// that need to be used by gob
func DeepCopy(src, copy interface{}, registeredTypes []interface{}) error {
	for _, t := range registeredTypes {
		gob.Register(t)
	}
	var buff bytes.Buffer
	enc := gob.NewEncoder(&buff)
	dec := gob.NewDecoder(&buff)
	err := enc.Encode(src)
	if err != nil {
		return errors.Wrap(err, "error encoding source")
	}
	return errors.Wrap(dec.Decode(copy), "error decoding copy")
}
