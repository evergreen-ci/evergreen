package model

import (
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/pkg/errors"
)

type APIPubKey struct {
	Name APIString `json:"name"`
	Key  APIString `json:"key"`
}

// BuildFromService converts from service level structs to an APIPubKey.
func (apiPubKey *APIPubKey) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case user.PubKey:
		apiPubKey.Name = APIString(v.Name)
		apiPubKey.Key = APIString(v.Key)
	default:
		return errors.Errorf("incorrect type when fetching converting pubkey type")
	}
	return nil
}

// ToService returns a service layer public key using the data from APIPubKey.
func (apiPubKey *APIPubKey) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIPubKey")
}
