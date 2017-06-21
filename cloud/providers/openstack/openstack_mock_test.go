package openstack

import (
	"fmt"
	"time"
	"errors"
	"math/rand"

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

// CreateInstance returns a mock server with an ID that is guaranteed to uniquely identify
// this server amongst all other mock servers.
func (c *clientMock) CreateInstance(_ servers.CreateOpts, _ string) (*servers.Server, error) {
	if c.failCreate {
		return nil, errors.New("failed to create instance")
	}

	server := &servers.Server{
		ID: fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int()),
	}
	return server, nil
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
