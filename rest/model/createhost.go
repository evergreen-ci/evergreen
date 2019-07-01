package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type CreateHost struct {
	DNSName    APIString `json:"dns_name"`
	IP         APIString `json:"ip_address"`
	InstanceID APIString `json:"instance_id"`

	HostID   APIString `json:"host_id"`
	ParentID APIString `json:"parent_id"`
	Image    APIString `json:"image"`
	Command  APIString `json:"command"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = ToAPIString(v.Id)
			createHost.ParentID = ToAPIString(v.ParentID)
			createHost.Image = ToAPIString(v.DockerOptions.Image)
			createHost.Command = ToAPIString(v.DockerOptions.Command)
			return nil
		}
		createHost.DNSName = ToAPIString(v.Host)
		createHost.InstanceID = ToAPIString(v.Id)
		createHost.IP = ToAPIString(v.IP)
	case *host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = ToAPIString(v.Id)
			createHost.ParentID = ToAPIString(v.ParentID)
			createHost.Image = ToAPIString(v.DockerOptions.Image)
			createHost.Command = ToAPIString(v.DockerOptions.Command)
			return nil
		}
		createHost.DNSName = ToAPIString(v.Host)
		createHost.InstanceID = ToAPIString(v.Id)
		createHost.IP = ToAPIString(v.IP)
	default:
		return errors.Errorf("Invalid type passed to *CreateHost.BuildFromService (%T)", h)
	}
	return nil
}

func (createHost *CreateHost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for CreateHost")
}
