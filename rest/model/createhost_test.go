package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestCreateHostBuildFromService(t *testing.T) {
	assert := assert.New(t)
	h := host.Host{
		Host:               "foo.com",
		ExternalIdentifier: "sir-123",
		IP:                 "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47",
		IPv4:               "12.34.56.78",
	}
	c := &APICreateHost{}
	c.BuildFromService(h)
	assert.Equal(utility.FromStringPtr(c.DNSName), h.Host)
	assert.Equal(utility.FromStringPtr(c.InstanceID), h.ExternalIdentifier)
	assert.Equal(utility.FromStringPtr(c.IP), h.IP)
	assert.Equal(utility.FromStringPtr(c.IPv4), h.IPv4)
}

func TestCreateHostBuildFromServiceWithContainer(t *testing.T) {
	assert := assert.New(t)
	h := host.Host{
		Id:       "i-1234",
		ParentID: "i-5678",
		DockerOptions: host.DockerOptions{
			Image:   "my-image",
			Command: "echo hi",
		},
		PortBindings: map[string][]string{
			"1234/tcp": {
				"98765",
			},
		},
	}
	c := &APICreateHost{}
	c.BuildFromService(h)
	assert.Equal(utility.FromStringPtr(c.HostID), h.Id)
	assert.Equal(utility.FromStringPtr(c.Image), h.DockerOptions.Image)
	assert.Equal(utility.FromStringPtr(c.Command), h.DockerOptions.Command)
	assert.Equal(utility.FromStringPtr(c.ParentID), h.ParentID)
	assert.Equal(c.PortBindings, h.PortBindings)

	assert.NotNil(c.DNSName) // will be set to parent's DNS name for host.list
	assert.Nil(c.InstanceID)
}
