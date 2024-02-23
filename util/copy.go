package util

import (
	"encoding/json"

	"github.com/pkg/errors"
)

// DeepCopy makes a deep copy of the src value into the copy params
func DeepCopy(src, copy interface{}) error {
	b, err := json.Marshal(src)
	if err != nil {
		return errors.Wrap(err, "marshalling source")
	}
	err = json.Unmarshal(b, copy)
	if err != nil {
		return errors.Wrap(err, "unmarshalling copy")
	}
	return nil
}
