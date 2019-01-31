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
		IPv6:               "1234::aba:asdf:a211:736",
	}
	c := &CreateHost{}
	err := c.BuildFromService(h)
	assert.NoError(err)
	assert.Equal(c.DNSName, h.Host)
	assert.Equal(c.InstanceID, h.ExternalIdentifier)
	assert.Equal(c.IPv6, h.IPv6)
}
