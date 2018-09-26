package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
)

func TestNewLDAPUserManager(t *testing.T) {
	conf := &evergreen.LDAPConfig{}
	u, err := NewLDAPUserManager(conf)
	assert.Error(t, err)
	assert.Nil(t, u)

	conf = &evergreen.LDAPConfig{
		URL:                "url",
		Port:               "port",
		UserPath:           "path",
		ServicePath:        "bot",
		Group:              "group",
		ExpireAfterMinutes: "60",
	}
	u, err = NewLDAPUserManager(conf)
	assert.NoError(t, err)
	assert.NotNil(t, u)
}
