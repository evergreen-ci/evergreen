package service

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestModifyHostStatusWithUpdateStatus(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx))
	require.NoError(db.ClearCollections(host.Collection, event.EventCollection), "error clearing collections")

	// Normal test, changing a host from running to quarantined
	t.Run("SuccessfullyModifiesHostStatusWithNote", func(t *testing.T) {
		user := user.DBUser{Id: "user"}
		h := host.Host{Id: "h1", Status: evergreen.HostRunning}
		require.NoError(h.Insert(ctx))
		opts := uiParams{Action: "updateStatus", Status: evergreen.HostQuarantined, Notes: "because I can"}

		result, httpStatus, err := api.ModifyHostStatus(ctx, env, &h, opts.Status, opts.Notes, &user)
		require.NoError(err)
		assert.Equal(http.StatusOK, httpStatus)
		assert.Equal(result, fmt.Sprintf(api.HostStatusUpdateSuccess, evergreen.HostRunning, evergreen.HostQuarantined))
		assert.Equal(h.Status, evergreen.HostQuarantined)
		events, err := event.Find(event.MostRecentHostEvents("h1", "", 1))
		assert.NoError(err)
		assert.Len(events, 1)
		hostevent, ok := events[0].Data.(*event.HostEventData)
		require.True(ok, "%T", events[0].Data)
		assert.Equal("because I can", hostevent.Logs)
	})
	t.Run("SuccessfullyUnquarantinesHostAndMarksAsReprovisioning", func(t *testing.T) {
		user := user.DBUser{Id: "user"}
		h := host.Host{
			Id:     "h2",
			Status: evergreen.HostQuarantined,
			Distro: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodSSH,
					Communication: distro.BootstrapMethodSSH,
				},
			},
			NumAgentCleanupFailures: 10,
		}
		require.NoError(h.Insert(ctx))

		_, httpStatus, err := api.ModifyHostStatus(ctx, env, &h, evergreen.HostRunning, "", &user)
		require.NoError(err)
		assert.Equal(http.StatusOK, httpStatus)
		assert.Equal(h.Status, evergreen.HostProvisioning)
		assert.Equal(host.ReprovisionToNew, h.NeedsReprovision)

		dbHost, err := host.FindOneId(ctx, h.Id)
		require.NoError(err)
		require.NotNil(t, dbHost)
		assert.Equal(0, dbHost.NumAgentCleanupFailures)
	})
	t.Run("FailsToDecommissionStaticHosts", func(t *testing.T) {
		user := user.DBUser{Id: "user"}
		h := host.Host{Id: "h3", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
		opts := uiParams{Action: "updateStatus", Status: evergreen.HostDecommissioned}

		_, _, err := api.ModifyHostStatus(ctx, env, &h, opts.Status, opts.Notes, &user)
		assert.Error(err)
		assert.Contains(err.Error(), api.DecommissionStaticHostError)
	})
	t.Run("FailsWithInvalidHostStatus", func(t *testing.T) {
		user := user.DBUser{Id: "user"}
		h := host.Host{Id: "h4", Status: evergreen.HostRunning, Provider: evergreen.ProviderNameStatic}
		opts := uiParams{Action: "updateStatus", Status: "undefined"}

		_, _, err := api.ModifyHostStatus(ctx, env, &h, opts.Status, opts.Notes, &user)
		assert.Error(err)
		assert.Contains(err.Error(), fmt.Sprintf(api.InvalidStatusError, "undefined"))
	})
}

func TestGetHostFromCache(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(host.Collection))
	uis := UIServer{hostCache: make(map[string]hostCacheItem)}

	// get a host that doesn't exist
	h, err := uis.getHostFromCache(ctx, "h1")
	assert.NoError(t, err)
	assert.Nil(t, h)

	// get a host from the cache
	uis.hostCache["h1"] = hostCacheItem{inserted: time.Now()}
	h, err = uis.getHostFromCache(ctx, "h1")
	assert.NoError(t, err)
	assert.NotNil(t, h)

	// past the TTL fetches from the db
	h1 := host.Host{Id: "h1", Host: "new_name"}
	assert.NoError(t, h1.Insert(ctx))
	uis.hostCache["h1"] = hostCacheItem{dnsName: "old_name", inserted: time.Now().Add(-1 * (hostCacheTTL + time.Second))}
	h, err = uis.getHostFromCache(ctx, "h1")
	assert.NoError(t, err)
	assert.NotNil(t, h)
	assert.Equal(t, "new_name", h.dnsName)
}

func TestGetHostDNS(t *testing.T) {
	r, err := http.NewRequest("GET", "", nil)
	require.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "i-1234"})

	uis := UIServer{hostCache: map[string]hostCacheItem{"i-1234": hostCacheItem{dnsName: "www.example.com", inserted: time.Now()}}}
	path, err := uis.getHostDNS((r))
	assert.NoError(t, err)
	assert.Len(t, path, 1)
	assert.Equal(t, fmt.Sprintf("www.example.com:%d", evergreen.VSCodePort), path[0])
}
