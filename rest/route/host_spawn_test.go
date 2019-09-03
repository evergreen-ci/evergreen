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
		Provider:     evergreen.ProviderNameEc2Auto,
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

	h.InstanceTags = map[string]string{
		"key": "value",
	}
	resp = h.Run(ctx)
	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())

	assert.Len(h.sc.(*data.MockConnector).MockHostConnector.CachedHosts, 3)
	h0 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[0]
	d0 := h0.Distro
	assert.Empty((*d0.ProviderSettings)["user_data"])
	assert.Empty(h0.InstanceTags)

	h1 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[1]
	d1 := h1.Distro
	assert.Equal("my script", (*d1.ProviderSettings)["user_data"].(string))
	assert.Empty(h1.InstanceTags)

	h2 := h.sc.(*data.MockConnector).MockHostConnector.CachedHosts[2]
	assert.Equal(map[string]string{"key": "value"}, h2.InstanceTags)
}
