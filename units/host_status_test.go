package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestCloudStatusJob(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	require.NoError(db.ClearCollections(host.Collection))
	hosts := []host.Host{
		{
			Id:       "host-1",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:         evergreen.ProviderNameMock,
				ProviderSettings: &map[string]interface{}{"region": "region-1"},
			},
		},
		{
			Id:       "host-2",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostStarting,
			Distro: distro.Distro{
				Provider:         evergreen.ProviderNameMock,
				ProviderSettings: &map[string]interface{}{"region": "region-2"},
			},
		},
		{
			Id:       "host-3",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostTerminated,
		},
		{
			Id:       "host-4",
			Provider: evergreen.ProviderNameMock,
			Status:   evergreen.HostProvisioning,
		},
	}
	for _, h := range hosts {
		require.NoError(h.Insert())
	}

	j := NewCloudHostReadyJob(&mock.Environment{}, "id")
	j.Run(context.Background())
	assert.NoError(j.Error())

	hosts, err := host.Find(db.Query(bson.M{}))
	assert.Len(hosts, 4)
	assert.NoError(err)
	for _, h := range hosts {
		if h.Id == "host-1" {
			assert.Equal(h.Status, evergreen.HostProvisioning)
		}
		if h.Id == "host-2" {
			assert.Equal(h.Status, evergreen.HostProvisioning)
		}
		if h.Id == "host-3" {
			assert.Equal(h.Status, evergreen.HostTerminated)
		}
		if h.Id == "host-4" {
			assert.Equal(h.Status, evergreen.HostProvisioning)
		}
	}
}
