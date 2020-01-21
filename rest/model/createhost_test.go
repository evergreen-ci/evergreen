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
	assert.Equal(FromStringPtr(c.DNSName), h.Host)
	assert.Equal(FromStringPtr(c.InstanceID), h.ExternalIdentifier)
	assert.Equal(FromStringPtr(c.IP), h.IP)
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
	}
	c := &CreateHost{}
	err := c.BuildFromService(h)
	assert.NoError(err)
	assert.Equal(FromStringPtr(c.Image), h.DockerOptions.Image)
	assert.Equal(FromStringPtr(c.Command), h.DockerOptions.Command)
	assert.Equal(FromStringPtr(c.ParentID), h.ParentID)

	assert.Nil(c.DNSName)
	assert.Nil(c.InstanceID)
}
