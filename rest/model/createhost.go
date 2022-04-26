package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type CreateHost struct {
	DNSName    *string `json:"dns_name,omitempty"`
	IP         *string `json:"ip_address,omitempty"`
	IPv4       *string `json:"ipv4_address,omitempty"`
	InstanceID *string `json:"instance_id,omitempty"`

	HostID       *string      `json:"host_id,omitempty"`
	ParentID     *string      `json:"parent_id,omitempty"`
	Image        *string      `json:"image,omitempty"`
	Command      *string      `json:"command,omitempty"`
	PortBindings host.PortMap `json:"port_bindings,omitempty"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		createHost.DNSName = utility.ToStringPtr(v.Host)
		createHost.IP = utility.ToStringPtr(v.IP)
		createHost.IPv4 = utility.ToStringPtr(v.IPv4)

		// container
		if v.ParentID != "" {
			createHost.HostID = utility.ToStringPtr(v.Id)
			createHost.ParentID = utility.ToStringPtr(v.ParentID)
			createHost.Image = utility.ToStringPtr(v.DockerOptions.Image)
			createHost.Command = utility.ToStringPtr(v.DockerOptions.Command)
			createHost.PortBindings = v.PortBindings
			return nil
		}
		createHost.InstanceID = utility.ToStringPtr(v.Id)
		if v.ExternalIdentifier != "" {
			createHost.InstanceID = utility.ToStringPtr(v.ExternalIdentifier)
		}
	case *host.Host:
		createHost.DNSName = utility.ToStringPtr(v.Host)
		createHost.IP = utility.ToStringPtr(v.IP)
		createHost.IPv4 = utility.ToStringPtr(v.IPv4)

		// container
		if v.ParentID != "" {
			createHost.HostID = utility.ToStringPtr(v.Id)
			createHost.ParentID = utility.ToStringPtr(v.ParentID)
			createHost.Image = utility.ToStringPtr(v.DockerOptions.Image)
			createHost.Command = utility.ToStringPtr(v.DockerOptions.Command)
			createHost.PortBindings = v.PortBindings
			return nil
		}
		createHost.InstanceID = utility.ToStringPtr(v.Id)
		if v.ExternalIdentifier != "" {
			createHost.InstanceID = utility.ToStringPtr(v.ExternalIdentifier)
		}
	default:
		return errors.Errorf("Invalid type passed to *CreateHost.BuildFromService (%T)", h)
	}
	return nil
}

func (createHost *CreateHost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for CreateHost")
}
