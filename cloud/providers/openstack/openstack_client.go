package openstack

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

// The client interface wraps the OpenStack client interaction.
type client interface {
	Init(gophercloud.AuthOptions, gophercloud.EndpointOpts) error
	CreateInstance(servers.CreateOpts) (*servers.Server, error)
	GetInstance(string) (*servers.Server, error)	
	DeleteInstance(string) error
}

type clientImpl struct {
	*gophercloud.ServiceClient
}

// Init establishes a connection to an Identity V3 endpoint and creates a client that
// can be used with the Compute V2 package.
func (c *clientImpl) Init(ao gophercloud.AuthOptions, eo gophercloud.EndpointOpts) error {
	providerClient, err := openstack.AuthenticatedClient(ao)
	if err != nil {
		return err
	}

	c.ServiceClient, err = openstack.NewComputeV2(providerClient, eo)
	return err
}

// CreateInstance requests a server to be provisioned to the user in the current tenant.
func (c *clientImpl) CreateInstance(opts servers.CreateOpts) (*servers.Server, error) {
	return servers.Create(c.ServiceClient, opts).Extract()
}	

// GetInstance requests details on a single server, by ID.
func (c *clientImpl) GetInstance(id string) (*servers.Server, error) {
	return servers.Get(c.ServiceClient, id).Extract()
}

// DeleteInstance requests a server previously provisioned to be removed, by ID.
func (c *clientImpl) DeleteInstance(id string) error {
	return servers.Delete(c.ServiceClient, id).ExtractErr()
}
