package model

import "github.com/pkg/errors"

type CreateHost struct {
	DNSName    string `json:"dns_name"`
	InstanceID string `json:"instance_id"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case *CreateHost:
		createHost.DNSName = v.DNSName
		createHost.InstanceID = v.InstanceID
	default:
		return errors.Errorf("Invalid type passed to *CreateHost.BuildFromService (%T)", h)
	}
	return nil
}

func (createHost *CreateHost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not impelemented for APIDistro")
}
