package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type CreateHost struct {
	DNSName    *string `json:"dns_name"`
	IP         *string `json:"ip_address"`
	InstanceID *string `json:"instance_id"`

	HostID   *string `json:"host_id"`
	ParentID *string `json:"parent_id"`
	Image    *string `json:"image"`
	Command  *string `json:"command"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = ToStringPtr(v.Id)
			createHost.ParentID = ToStringPtr(v.ParentID)
			createHost.Image = ToStringPtr(v.DockerOptions.Image)
			createHost.Command = ToStringPtr(v.DockerOptions.Command)
			return nil
		}
		createHost.DNSName = ToStringPtr(v.Host)
		createHost.InstanceID = ToStringPtr(v.Id)
		createHost.IP = ToStringPtr(v.IP)
		createHost.InstanceID = ToStringPtr(v.ExternalIdentifier)
	case *host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = ToStringPtr(v.Id)
			createHost.ParentID = ToStringPtr(v.ParentID)
			createHost.Image = ToStringPtr(v.DockerOptions.Image)
			createHost.Command = ToStringPtr(v.DockerOptions.Command)
			return nil
		}
		createHost.DNSName = ToStringPtr(v.Host)
		createHost.InstanceID = ToStringPtr(v.Id)
		createHost.IP = ToStringPtr(v.IP)
		createHost.InstanceID = ToStringPtr(v.ExternalIdentifier)
	default:
		return errors.Errorf("Invalid type passed to *CreateHost.BuildFromService (%T)", h)
	}
	return nil
}

func (createHost *CreateHost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for CreateHost")
}
