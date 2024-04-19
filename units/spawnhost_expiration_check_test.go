package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnhostExpirationCheckJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	config := testutil.TestConfig()
	assert.NoError(t, evergreen.UpdateConfig(ctx, config))
	assert.NoError(t, db.ClearCollections(host.Collection))
	mock := cloud.GetMockProvider()

	h := host.Host{
		Id:       "test-host",
		Status:   evergreen.HostRunning,
		UserHost: true,
		Provider: evergreen.ProviderNameMock,
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameMock,
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("region", "test-region"),
			)},
		},
		NoExpiration:   true,
		ExpirationTime: time.Now(),
	}

	assert.NoError(t, h.Insert(ctx))
	mock.Set(h.Id, cloud.MockInstance{
		Status: cloud.StatusRunning,
	})

	ts := utility.RoundPartOfHour(0).Format(TSFormat)
	j := NewSpawnhostExpirationCheckJob(ts, &h)
	j.Run(context.Background())
	assert.NoError(t, j.Error())
	assert.True(t, j.Status().Completed)

	found, err := host.FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	require.NotNil(t, found)
	assert.True(t, found.ExpirationTime.Sub(h.ExpirationTime) > 0)
}

func TestTryIdleSpawnHostNotification(t *testing.T) {
	assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, event.EventCollection, user.Collection))
	h := &host.Host{
		Id:       "test-host",
		Status:   evergreen.HostStopped,
		UserHost: true,
		Provider: evergreen.ProviderNameMock,
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameMock,
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("region", "test-region"),
			)},
		},
		SleepSchedule: host.SleepScheduleInfo{
			ShouldKeepOff: true,
		},
		NoExpiration:          true,
		ExpirationTime:        time.Now(),
		LastCommunicationTime: time.Now().Add(-time.Hour * 24 * 365),
		StartedBy:             "me",
	}
	u := &user.DBUser{
		Id:           "me",
		EmailAddress: "me.ee@ee.com",
	}
	assert.NoError(t, u.Insert())
	assert.NoError(t, tryIdleSpawnHostNotification(h))

	fetchedSubs := []event.Subscription{}
	assert.NoError(t, db.FindAllQ(event.SubscriptionsCollection, db.Q{}, &fetchedSubs))
	require.Len(t, fetchedSubs, 1)
	assert.Equal(t, u.EmailAddress, utility.FromStringPtr(fetchedSubs[0].Subscriber.Target.(*string)))
	assert.Equal(t, event.EmailSubscriberType, fetchedSubs[0].Subscriber.Type)
	assert.Equal(t, fetchedSubs[0].ID, h.Id)

	// Trying to re-insert the subscription doesn't create a new subscription.
	assert.NoError(t, tryIdleSpawnHostNotification(h))
	fetchedSubs = []event.Subscription{}
	assert.NoError(t, db.FindAllQ(event.SubscriptionsCollection, db.Q{}, &fetchedSubs))
	require.Len(t, fetchedSubs, 1)

	events, err := event.FindAllByResourceID(h.Id)
	assert.NoError(t, err)
	assert.Len(t, events, 2)
}
