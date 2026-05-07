package units

import (
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTaskHostExpirationExtendJob(t *testing.T) {
	ctx := testutil.TestSpan(t.Context(), t)

	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(ctx, config))

	expireOn := time.Now().Format(evergreen.ExpireOnFormat)
	makeHost := func(id string) host.Host {
		return host.Host{
			Id:        id,
			Status:    evergreen.HostRunning,
			UserHost:  false,
			StartedBy: evergreen.User,
			Provider:  evergreen.ProviderNameMock,
			Distro: distro.Distro{
				Provider: evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(
					birch.EC.String("region", "test-region"),
				)},
			},
			InstanceTags: []host.Tag{
				{Key: evergreen.TagExpireOn, Value: expireOn, CanBeModified: false},
			},
		}
	}

	t.Run("ExtendsExpireOnTagByOneDay", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(host.Collection))
		mock := cloud.GetMockProvider()

		h := makeHost("test-host")
		require.NoError(t, h.Insert(ctx))
		mock.Set(h.Id, cloud.MockInstance{
			Status: cloud.StatusRunning,
			Tags:   h.InstanceTags,
		})

		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		j := NewTaskHostExpirationExtendJob(ts, &h)
		j.Run(ctx)
		assert.NoError(t, j.Error())

		found, err := host.FindOneId(ctx, h.Id)
		require.NoError(t, err)
		require.NotNil(t, found)

		expectedExpireOn := time.Now().AddDate(0, 0, 1).Format(evergreen.ExpireOnFormat)
		var gotExpireOn string
		for _, tag := range found.InstanceTags {
			if tag.Key == evergreen.TagExpireOn {
				gotExpireOn = tag.Value
				break
			}
		}
		assert.Equal(t, expectedExpireOn, gotExpireOn)
	})

	t.Run("HostNotFoundErrors", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(host.Collection))

		h := makeHost("nonexistent-host")
		ts := utility.RoundPartOfHour(0).Format(TSFormat)
		j := NewTaskHostExpirationExtendJob(ts, &h)
		j.Run(ctx)
		assert.Error(t, j.Error())
	})
}
