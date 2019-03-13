package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/pkg/errors"
)

type CreateHost struct {
	DNSName    string `json:"dns_name"`
	IP         string `json:"ip_address"`
	InstanceID string `json:"instance_id"`

	ContainerID string `json:"container_id"`
	ParentID    string `json:"parent_id"`
	Image       string `json:"image"`
	Command     string `json:"command"`
	Port        int    `json:"port"`
}

func (createHost *CreateHost) BuildFromService(h interface{}) error {
	switch v := h.(type) {
	case host.Host:
		// container
		if v.ParentID != "" {
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
