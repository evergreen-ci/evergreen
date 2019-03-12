package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestCreateHostBuildFromService(t *testing.T) {
	assert := assert.New(t)
	h := host.Host{
		Host:               "foo.com",
		ExternalIdentifier: "sir-123",
		IP:                 "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47",
	}
	c := &CreateHost{}
	err := c.BuildFromService(h)
	assert.NoError(err)
	assert.Equal(c.DNSName, h.Host)
	assert.Equal(c.InstanceID, h.ExternalIdentifier)
	assert.Equal(c.IP, h.IP)
}

func TestCreateHostBuildFromServiceWithContainer(t *testing.T) {
	assert := assert.New(t)
	h := host.Host{
		Id:                 "i-1234",
		ExternalIdentifier: "container-1234",
		ParentID:           "i-5678",
		DockerOptions: host.DockerOptions{
			Image:   "my-image",
			Command: "echo hi",
		},
	}
	c := &CreateHost{}
	err := c.BuildFromService(h)
	assert.NoError(err)
	assert.Equal(c.Image, h.DockerOptions.Image)
	assert.Equal(c.Command, h.DockerOptions.Command)
	assert.Equal(c.ContainerID, h.ExternalIdentifier)
	assert.Equal(c.Host, h.Id)

	assert.Empty(c.DNSName)
	assert.Empty(c.InstanceID)
}
