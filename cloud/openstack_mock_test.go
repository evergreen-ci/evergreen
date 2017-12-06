package cloud

import (
	"errors"
	"fmt"
	"math/rand"
	"time"

	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack/compute/v2/servers"
)

type openStackClientMock struct {
	// API call options
	failInit   bool
	failCreate bool
	failGet    bool
	failDelete bool

	// Other options
	isServerActive bool
}

func (c *openStackClientMock) Init(_ gophercloud.AuthOptions, _ gophercloud.EndpointOpts) error {
	if c.failInit {
		return errors.New("failed to initialize client")
	}

	return nil
}

// CreateInstance returns a mock server with an ID that is guaranteed to uniquely identify
// this server amongst all other mock servers.
func (c *openStackClientMock) CreateInstance(_ servers.CreateOpts, _ string) (*servers.Server, error) {
	if c.failCreate {
		return nil, errors.New("failed to create instance")
	}

	server := &servers.Server{
		ID: fmt.Sprintf("_%v", rand.New(rand.NewSource(time.Now().UnixNano())).Int()),
	}
	return server, nil
}

func (c *openStackClientMock) GetInstance(id string) (*servers.Server, error) {
	if c.failGet {
		return nil, errors.New("failed to get instance")
	}

	server := &servers.Server{
		Status: "ACTIVE",
		Addresses: map[string]interface{}{
			"subnet": []interface{}{
				map[string]interface{}{"addr": "0.0.0.0"},
			},
		},
	}

	if !c.isServerActive {
		server.Status = "SHUTOFF"
	}

	return server, nil
}

func (c *openStackClientMock) DeleteInstance(id string) error {
	if c.failDelete {
		return errors.New("failed to delete instance")
	}

	return nil
}
