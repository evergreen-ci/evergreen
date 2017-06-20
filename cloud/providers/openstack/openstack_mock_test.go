package openstack

import (
	"errors"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type clientMock struct {
	// API call options
	failInit   bool
	failCreate bool
	failGet    bool
	failDelete bool

	// Other options
	isServerActive bool
}

func (c *clientMock) Init(_ gophercloud.AuthOptions, _ gophercloud.EndpointOpts) error {
	if c.failInit {
		return errors.New("failed to initialize client")
	}

	return nil
}

func (c *clientMock) CreateInstance(_ servers.CreateOpts, _ string) (*servers.Server, error) {
	if c.failCreate {
		return nil, errors.New("failed to create instance")
	}

	return &servers.Server{ID: "id"}, nil
}

func (c *clientMock) GetInstance(id string) (*servers.Server, error) {
	if c.failGet {
		return nil, errors.New("failed to get instance")
	}

	server := &servers.Server{Status: "ACTIVE"}
	if !c.isServerActive {
		server.Status = "SHUTOFF"
	}

	return server, nil
}

func (c *clientMock) DeleteInstance(id string) error {
	if c.failDelete {
		return errors.New("failed to delete instance")
	}

	return nil
}
