package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
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
	mock := cloud.GetMockProvider()
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
		InstanceType: "instance-type-1",
		Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
	}
	assert.NoError(t, h.Insert())
	mock.Set(h.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
		Tags: []host.Tag{
			host.Tag{
				Key:           "key1",
				Value:         "value1",
				CanBeModified: true,
			},
		},
		Type: "instance-type-1",
	})

	changes := host.HostModifyOptions{
		AddInstanceTags: []host.Tag{
			host.Tag{
				Key:           "key2",
				Value:         "value2",
				CanBeModified: true,
			},
		},
		DeleteInstanceTags: []string{"key1"},
		InstanceType:       "instance-type-2",
	}

	ts := util.RoundPartOfMinute(1).Format(TSFormat)
	j := NewSpawnhostModifyJob(&h, changes, ts)

	j.Run(context.Background())
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	modifiedHost, err := host.FindOneId(h.Id)
	assert.NoError(t, err)
	assert.Equal(t, []host.Tag{host.Tag{Key: "key2", Value: "value2", CanBeModified: true}}, modifiedHost.InstanceTags)
	assert.Equal(t, "instance-type-2", modifiedHost.InstanceType)
}
