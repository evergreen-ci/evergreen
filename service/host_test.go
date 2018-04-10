package service

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
)

func TestModifyHostStatus(t *testing.T) {
	assert := assert.New(t)

	// Normal test, changing a host from running to quarantined
	user1 := user.DBUser{Id: "user1"}
	h1 := host.Host{Id: "h1", Status: evergreen.HostRunning}
	opts1 := uiParams{Action: "updateStatus", Status: evergreen.HostQuarantined}

	result, err := modifyHostStatus(&h1, &opts1, &user1)
	assert.Nil(err)
	assert.Equal(result, fmt.Sprintf(HostStatusUpdateSuccess, evergreen.HostRunning, evergreen.HostQuarantined))
	assert.Equal(h1.Status, evergreen.HostQuarantined)

	user2 := user.DBUser{Id: "user2"}
	h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
	opts2 := uiParams{Action: "updateStatus", Status: evergreen.HostDecommissioned}

	_, err = modifyHostStatus(&h2, &opts2, &user2)
	assert.NotNil(err)
	assert.Equal(err.Error(), DecommissionStaticHostError)

	user3 := user.DBUser{Id: "user3"}
	h3 := host.Host{Id: "h3", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
	opts3 := uiParams{Action: "updateStatus", Status: "undefined"}

	_, err = modifyHostStatus(&h3, &opts3, &user3)
	assert.NotNil(err)
	assert.Equal(err.Error(), fmt.Sprintf(InvalidStatusError, "undefined"))
}
