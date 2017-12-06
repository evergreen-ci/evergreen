package cloud

import (
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/extensions/keypairs"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
	"github.com/pkg/errors"
)

// The openStackClient interface wraps the OpenStack openStackClient interaction.
type openStackClient interface {
	Init(gophercloud.AuthOptions, gophercloud.EndpointOpts) error
	CreateInstance(servers.CreateOpts, string) (*servers.Server, error)
	GetInstance(string) (*servers.Server, error)
	DeleteInstance(string) error
}

type openStackClientImpl struct {
	*gophercloud.ServiceClient
}

// Init establishes a connection to an Identity V3 endpoint and creates a openStackClient that
// can be used with the Compute V2 package.
func (c *openStackClientImpl) Init(ao gophercloud.AuthOptions, eo gophercloud.EndpointOpts) error {
	providerClient, err := openstack.AuthenticatedClient(ao)
	if err != nil {
		return errors.Wrap(err, "OpenStack AuthenticatedClient API call failed")
	}

	c.ServiceClient, err = openstack.NewComputeV2(providerClient, eo)
	if err != nil {
		return errors.Wrap(err, "OpenStack NewComputeV2 API call failed")
	}
	return nil
}

// CreateInstance requests a server to be provisioned to the user in the current tenant.
func (c *openStackClientImpl) CreateInstance(opts servers.CreateOpts, keyName string) (*servers.Server, error) {
	opts.ServiceClient = c.ServiceClient
	optsExt := keypairs.CreateOptsExt{
		CreateOptsBuilder: opts,
		KeyName:           keyName,
	}
	server, err := servers.Create(c.ServiceClient, optsExt).Extract()
	return server, errors.Wrap(err, "OpenStack Create API call failed")
}

// GetInstance requests details on a single server, by ID.
func (c *openStackClientImpl) GetInstance(id string) (*servers.Server, error) {
	server, err := servers.Get(c.ServiceClient, id).Extract()
	return server, errors.Wrap(err, "OpenStack Get API call failed")
}

// DeleteInstance requests a server previously provisioned to be removed, by ID.
func (c *openStackClientImpl) DeleteInstance(id string) error {
	err := servers.Delete(c.ServiceClient, id).ExtractErr()
	return errors.Wrap(err, "OpenStack Delete API call failed")
}
