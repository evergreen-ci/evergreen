package service

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModifyHostStatusWithUpdateStatus(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := mock.Environment{}
	assert.NoError(env.Configure(ctx))
	require.NoError(db.ClearCollections(event.AllLogCollection), "error clearing collections")

	// Normal test, changing a host from running to quarantined
	user1 := user.DBUser{Id: "user1"}
	h1 := host.Host{Id: "h1", Status: evergreen.HostRunning}
	opts1 := uiParams{Action: "updateStatus", Status: evergreen.HostQuarantined, Notes: "because I can"}

	result, err := modifyHostStatus(env.LocalQueue(), &h1, &opts1, &user1)
	assert.NoError(err)
	assert.Equal(result, fmt.Sprintf(HostStatusUpdateSuccess, evergreen.HostRunning, evergreen.HostQuarantined))
	assert.Equal(h1.Status, evergreen.HostQuarantined)
	events, err2 := event.Find(event.AllLogCollection, event.MostRecentHostEvents("h1", "", 1))
	assert.NoError(err2)
	assert.Len(events, 1)
	hostevent, ok := events[0].Data.(*event.HostEventData)
	require.True(ok, "%T", events[0].Data)
	assert.Equal("because I can", hostevent.Logs)

	user2 := user.DBUser{Id: "user2"}
	h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
	opts2 := uiParams{Action: "updateStatus", Status: evergreen.HostDecommissioned}

	_, err = modifyHostStatus(env.LocalQueue(), &h2, &opts2, &user2)
	assert.Error(err)
	assert.Contains(err.Error(), DecommissionStaticHostError)

	user3 := user.DBUser{Id: "user3"}
	h3 := host.Host{Id: "h3", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
	opts3 := uiParams{Action: "updateStatus", Status: "undefined"}

	_, err = modifyHostStatus(env.LocalQueue(), &h3, &opts3, &user3)
	assert.Error(err)
	assert.Contains(err.Error(), fmt.Sprintf(InvalidStatusError, "undefined"))
}
