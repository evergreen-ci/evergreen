package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
)

type APICreateHost struct {
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

func (createHost *APICreateHost) BuildFromService(h host.Host) {
	createHost.DNSName = utility.ToStringPtr(h.Host)
	createHost.IP = utility.ToStringPtr(h.IP)
	createHost.IPv4 = utility.ToStringPtr(h.IPv4)

	// container
	if h.ParentID != "" {
		createHost.HostID = utility.ToStringPtr(h.Id)
		createHost.ParentID = utility.ToStringPtr(h.ParentID)
		createHost.Image = utility.ToStringPtr(h.DockerOptions.Image)
		createHost.Command = utility.ToStringPtr(h.DockerOptions.Command)
		createHost.PortBindings = h.PortBindings
	}
	createHost.InstanceID = utility.ToStringPtr(h.Id)
	if h.ExternalIdentifier != "" {
		createHost.InstanceID = utility.ToStringPtr(h.ExternalIdentifier)
	}
}
