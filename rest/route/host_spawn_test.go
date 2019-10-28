package route

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
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
	settings, err := evergreen.GetConfig()
	assert.NoError(err)
	h := &hostPostHandler{
		settings: settings,
		options: &model.HostRequestOptions{
			TaskID:   "task",
			DistroID: "distro",
			KeyName:  "keyname",
		},
	}
	h.sc = &data.MockConnector{}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})

	resp := h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	h.options.UserData = "my script"
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	h.options.InstanceTags = []host.Tag{
		host.Tag{
			Key:           "key",
			Value:         "value",
			CanBeModified: true,
		},
	}
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	h.settings.Providers.AWS.AllowedInstanceTypes = append(h.settings.Providers.AWS.AllowedInstanceTypes, "test_instance_type")
	h.options.InstanceType = "test_instance_type"
	resp = h.Run(ctx)
	require.NotNil(resp)
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
	h := &hostStopHandler{
		sc:  &data.MockConnector{},
		env: evergreen.GetEnvironment(),
	}
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
	h := &hostStartHandler{
		sc:  &data.MockConnector{},
		env: evergreen.GetEnvironment(),
	}
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

func TestAttachVolumeHandler(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.VolumesCollection))
	h := &attachVolumeHandler{
		sc:  &data.MockConnector{},
		env: evergreen.GetEnvironment(),
	}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})
	h.sc.(*data.MockConnector).MockHostConnector.CachedHosts = []host.Host{
		host.Host{
			Id:        "my-host",
			Status:    evergreen.HostRunning,
			StartedBy: "user",
			Zone:      "us-east-1c",
		},
		host.Host{
			Id: "different-host",
		},
	}

	// no volume
	v := &host.VolumeAttachment{DeviceName: "my-device"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest("GET", "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.Error(t, h.Parse(ctx, r))

	// wrong availability zone
	v.VolumeID = "my-volume"
	volume := &host.Volume{
		ID: v.VolumeID,
	}
	assert.NoError(t, volume.Insert())

	jsonBody, err = json.Marshal(v)
	assert.NoError(t, err)
	buffer = bytes.NewBuffer(jsonBody)

	r, err = http.NewRequest("GET", "/hosts/my-host/attach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))

	require.NotNil(t, h.attachment)
	assert.Equal(t, h.attachment.VolumeID, "my-volume")
	assert.Equal(t, h.attachment.DeviceName, "my-device")

	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusBadRequest, resp.Status())
}

func TestDetachVolumeHandler(t *testing.T) {
	h := &detachVolumeHandler{
		sc:  &data.MockConnector{},
		env: evergreen.GetEnvironment(),
	}
	ctx := gimlet.AttachUser(context.Background(), &user.DBUser{Id: "user"})

	h.sc.(*data.MockConnector).MockHostConnector.CachedHosts = []host.Host{
		host.Host{
			Id:        "my-host",
			StartedBy: "user",
			Status:    evergreen.HostRunning,
			Volumes: []host.VolumeAttachment{
				{
					VolumeID:   "my-volume",
					DeviceName: "my-device",
				},
			},
		},
	}

	v := host.VolumeAttachment{VolumeID: "not-a-volume"}
	jsonBody, err := json.Marshal(v)
	assert.NoError(t, err)
	buffer := bytes.NewBuffer(jsonBody)

	r, err := http.NewRequest("GET", "/hosts/my-host/detach", buffer)
	assert.NoError(t, err)
	r = gimlet.SetURLVars(r, map[string]string{"host_id": "my-host"})

	assert.NoError(t, h.Parse(ctx, r))
	resp := h.Run(ctx)
	assert.NotNil(t, resp)
	assert.Equal(t, http.StatusNotFound, resp.Status())
}
