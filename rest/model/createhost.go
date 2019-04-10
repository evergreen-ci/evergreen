package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type CreateHost struct {
	DNSName    string `json:"dns_name,omitempty"`
	IP         string `json:"ip_address,omitempty"`
	InstanceID string `json:"instance_id,omitempty"`

	HostID      string `json:"host_id,omitempty"`
	ContainerID string `json:"container_id,omitempty"`
	ParentID    string `json:"parent_id,omitempty"`
	Image       string `json:"image,omitempty"`
	Command     string `json:"command,omitempty"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = v.Id
			createHost.ContainerID = v.ExternalIdentifier
			createHost.ParentID = v.ParentID
			createHost.Image = v.DockerOptions.Image
			createHost.Command = v.DockerOptions.Command
			return nil
		}
		createHost.DNSName = v.Host
		createHost.InstanceID = v.Id
		createHost.IP = v.IP
		createHost.InstanceID = v.ExternalIdentifier
	case *host.Host:
		// container
		if v.ParentID != "" {
			createHost.HostID = v.Id
			createHost.ContainerID = v.ExternalIdentifier
			createHost.ParentID = v.ParentID
			createHost.Image = v.DockerOptions.Image
			createHost.Command = v.DockerOptions.Command
			return nil
		}
		createHost.DNSName = v.Host
		createHost.InstanceID = v.Id
		createHost.IP = v.IP
		createHost.InstanceID = v.ExternalIdentifier
	default:
		return errors.Errorf("Invalid type passed to *CreateHost.BuildFromService (%T)", h)
	}
	return nil
}

func (createHost *CreateHost) ToService() (interface{}, error) {
	return nil, errors.Errorf("ToService() is not implemented for CreateHost")
}
