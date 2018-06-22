package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateHostBuildFromService(t *testing.T) {
	assert := assert.New(t)
	c := &CreateHost{
		DNSName:    "foo.com",
		InstanceID: "sir-123",
	}
	createHost := &CreateHost{}
	err := createHost.BuildFromService(c)
	assert.NoError(err)
	assert.Equal(c, createHost)
}
