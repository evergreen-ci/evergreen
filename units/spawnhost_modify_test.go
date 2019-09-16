package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
)

func TestSpawnhostModifyJob(t *testing.T) {
	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	h := host.Host{
		Id:       "hostID",
		Provider: evergreen.ProviderNameMock,
		InstanceTags: []host.Tag{
			host.Tag{
				Key:           "key1",
				Value:         "value1",
				CanBeModified: true,
			},
		},
		Distro: distro.Distro{Provider: evergreen.ProviderNameMock},
	}
	assert.NoError(t, h.Insert())

	changes := host.HostModifyOptions{
		AddInstanceTags: []host.Tag{
			host.Tag{
				Key:           "key2",
				Value:         "value2",
				CanBeModified: true,
			},
		},
		DeleteInstanceTags: []string{"key1"},
	}

	ts := util.GetUTCSecond(time.Now()).Format(tsFormat)
	j := NewSpawnhostModifyJob(&h, changes, ts)

	j.Run(context.Background())
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	modifiedHost, err := host.FindOneId(h.Id)
	assert.NoError(t, err)
	assert.Equal(t, []host.Tag{host.Tag{Key: "key2", Value: "value2", CanBeModified: true}}, modifiedHost.InstanceTags)
}
