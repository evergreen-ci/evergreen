package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostPostHandler(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(distro.Collection, host.Collection))

	d := &distro.Distro{
		Id:           "distro",
		SpawnAllowed: true,
		Provider:     evergreen.ProviderNameEc2OnDemand,
	}
	require.NoError(d.Insert())
	h := &hostPostHandler{
		Task:    "task",
		Distro:  "distro",
		KeyName: "keyname",
	}
	h.sc = &data.MockConnector{}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})

	resp := h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h.UserData = "my script"
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h.InstanceTags = []host.Tag{
		host.Tag{
			Key:           "key",
			Value:         "value",
			CanBeModified: true,
		},
	}
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h.InstanceType = "test_instance_type"
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	assert.Len(h.sc.(*data.MockConnector).MockHostConnector.CachedHosts, 4)
	h0 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[0]
	d0 := h0.Distro
	assert.Empty((*d0.ProviderSettings)["user_data"])
	assert.Empty(h0.InstanceTags)
	assert.Empty(h0.InstanceType)

	h1 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[1]
	d1 := h1.Distro
	assert.Equal("my script", (*d1.ProviderSettings)["user_data"].(string))
	assert.Empty(h1.InstanceTags)
	assert.Empty(h1.InstanceType)

	h2 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[2]
	assert.Equal([]host.Tag{host.Tag{Key: "key", Value: "value", CanBeModified: true}}, h2.InstanceTags)
	assert.Empty(h2.InstanceType)

	h3 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[3]
	assert.Equal("test_instance_type", h3.InstanceType)
}

func TestHostStopHandler(t *testing.T) {
	h := &hostStopHandler{sc: &data.MockConnector{}}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})

	h.sc.(*data.MockConnector).MockHostConnector.CachedHosts = []host.Host{
		host.Host{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
		host.Host{
			Id:     "host-provisioning",
			Status: evergreen.HostProvisioning,
		},
		host.Host{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
	}

	h.hostID = "host-stopped"
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = "host-provisioning"
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = "host-running"
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}

func TestHostStartHandler(t *testing.T) {
	h := &hostStartHandler{sc: &data.MockConnector{}}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})

	h.sc.(*data.MockConnector).MockHostConnector.CachedHosts = []host.Host{
		host.Host{
			Id:     "host-running",
			Status: evergreen.HostRunning,
		},
		host.Host{
			Id:     "host-stopped",
			Status: evergreen.HostStopped,
		},
	}

	h.hostID = "host-running"
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())

	h.hostID = "host-stopped"
	resp = h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusOK, resp.Status())
}
