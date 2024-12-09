package trigger

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestVolumeTriggers(t *testing.T) {
	suite.Run(t, &hostSuite{})
}

func TestVolumeExpiration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.Implements(t, (*eventHandler)(nil), &volumeTriggers{})

	require.NoError(t, db.ClearCollections(event.EventCollection, host.VolumesCollection, event.SubscriptionsCollection, alertrecord.Collection))
	v := host.Volume{
		ID:         "v0",
		Expiration: time.Now().Add(12 * time.Hour),
	}
	require.NoError(t, v.Insert())

	triggers := makeVolumeTriggers().(*volumeTriggers)
	triggers.volume = &v
	triggers.event = &event.EventLogEntry{ID: "e0"}

	testData := hostTemplateData{
		ID: v.ID,
	}

	uiConfig := &evergreen.UIConfig{
		Url:     "https://evergreen.mongodb.com",
		UIv2Url: "https://spruce.mongodb.com",
	}
	require.NoError(t, uiConfig.Set(ctx))

	for name, test := range map[string]func(*testing.T){
		"Email": func(*testing.T) {
			email, err := testData.hostEmailPayload(expiringVolumeEmailSubject, expiringVolumeEmailBody, triggers.Attributes())
			assert.NoError(t, err)
			assert.Contains(t, email.Body, "Your volume with id v0 is unattached and will be terminated at")
		},
		"Slack": func(*testing.T) {
			slack, err := testData.hostSlackPayload(expiringVolumeSlackBody, "linkTitle")
			assert.NoError(t, err)
			assert.Contains(t, slack.Body, "Your volume with id v0 is unattached and will be terminated at")
		},
		"Fetch": func(*testing.T) {
			triggers := volumeTriggers{}
			assert.NoError(t, triggers.Fetch(ctx, &event.EventLogEntry{ResourceId: "v0"}))
			assert.Equal(t, "v0", triggers.templateData.ID)
			assert.Equal(t, fmt.Sprintf("%s/spawn/volume", uiConfig.UIv2Url), triggers.templateData.URL)
		},
		"NotificationsFromEvent": func(*testing.T) {
			require.NoError(t, db.Clear(event.SubscriptionsCollection))
			subscriptions := []event.Subscription{
				event.NewSubscriptionByID(event.ResourceTypeHost, event.TriggerExpiration, v.ID, event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "foo@bar.com",
				}),
			}
			require.NoError(t, subscriptions[0].Upsert())

			n, err := NotificationsFromEvent(ctx, &event.EventLogEntry{
				ResourceType: event.ResourceTypeHost,
				EventType:    event.EventVolumeExpirationWarningSent,
				ResourceId:   v.ID,
				Data:         &event.HostEventData{},
			})
			assert.NoError(t, err)
			assert.Len(t, n, 1)
		},
		"VolumeExpiration": func(*testing.T) {
			sub := &event.Subscription{}
			sub.Subscriber = event.Subscriber{Type: event.EmailSubscriberType}
			sub.Selectors = []event.Selector{{Type: event.SelectorID, Data: v.ID}}
			sub.Trigger = "t"
			n, err := triggers.volumeExpiration(ctx, sub)
			assert.NoError(t, err)
			assert.NotNil(t, n)
		},
	} {
		require.NoError(t, db.Clear(alertrecord.Collection))
		t.Run(name, test)
	}
}
