package model

import (
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
)

type APICreateHost struct {
	DNSName    *string `json:"dns_name"`
	IP         *string `json:"ip_address"`
	IPv4       *string `json:"ipv4_address"`
	InstanceID *string `json:"instance_id"`

	HostID       *string      `json:"host_id"`
	ParentID     *string      `json:"parent_id"`
	Image        *string      `json:"image"`
	Command      *string      `json:"command"`
	PortBindings host.PortMap `json:"port_bindings"`
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
		return
	}
	createHost.InstanceID = utility.ToStringPtr(h.Id)
	if h.ExternalIdentifier != "" {
		createHost.InstanceID = utility.ToStringPtr(h.ExternalIdentifier)
	}
}
