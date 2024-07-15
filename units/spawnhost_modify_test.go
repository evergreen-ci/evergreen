package units

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func checkSpawnHostModificationEvent(t *testing.T, hostID, expectedEvent string, expectedSuccess bool) {
	events, err := event.FindAllByResourceID(hostID)
	require.NoError(t, err)

	var foundEvent bool
	for _, e := range events {
		if e.EventType == expectedEvent {
			hostData, ok := e.Data.(*event.HostEventData)
			require.True(t, ok)

			assert.Equal(t, expectedSuccess, hostData.Successful)
			if !expectedSuccess {
				assert.NotEmpty(t, hostData.Logs)
			}

			foundEvent = true

			break
		}
	}
	assert.True(t, foundEvent, "event '%s' should be logged", expectedEvent)
}

func TestSpawnhostModifyJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	defer func() {
		assert.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))
	}()

	t.Run("NewSpawnhostModifyJobSetsExpectedFields", func(t *testing.T) {
		ts := utility.RoundPartOfMinute(1).Format(TSFormat)
		h := host.Host{
			Id:       "host_id",
			Status:   evergreen.HostRunning,
			Provider: evergreen.ProviderNameMock,
			Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
		}
		modifyOpts := host.HostModifyOptions{
			InstanceType: "m4.4xlarge",
		}
		j, ok := NewSpawnhostModifyJob(&h, modifyOpts, ts).(*spawnhostModifyJob)
		require.True(t, ok)

		assert.Equal(t, h.Id, j.HostID)
		assert.Equal(t, j.ModifyOptions, modifyOpts)
		assert.Equal(t, evergreen.ModifySpawnHostManual, j.Source)
	})

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *host.Host, mock cloud.MockProvider){
		"RunModifiesInstanceType": func(ctx context.Context, t *testing.T, h *host.Host, mock cloud.MockProvider) {
			require.NoError(t, h.Insert(ctx))
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
				Tags: []host.Tag{
					{
						Key:           "key1",
						Value:         "value1",
						CanBeModified: true,
					},
				},
				Type: "instance-type-1",
			})
			changes := host.HostModifyOptions{
				AddInstanceTags: []host.Tag{
					{
						Key:           "key2",
						Value:         "value2",
						CanBeModified: true,
					},
				},
				DeleteInstanceTags: []string{"key1"},
				InstanceType:       "instance-type-2",
			}

			j := NewSpawnhostModifyJob(h, changes, "")
			j.Run(ctx)
			assert.NoError(t, j.Error())
			assert.True(t, j.Status().Completed)

			modifiedHost, err := host.FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, modifiedHost)
			assert.Equal(t, []host.Tag{{Key: "key2", Value: "value2", CanBeModified: true}}, modifiedHost.InstanceTags)
			assert.Equal(t, "instance-type-2", modifiedHost.InstanceType)

			checkSpawnHostModificationEvent(t, h.Id, event.EventHostModified, true)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			require.NoError(t, db.ClearCollections(host.Collection, event.EventCollection))

			mock := cloud.GetMockProvider()
			h := host.Host{
				Id:       "host_id",
				Status:   evergreen.HostRunning,
				Provider: evergreen.ProviderNameMock,
				Distro:   distro.Distro{Provider: evergreen.ProviderNameMock},
			}

			tCase(ctx, t, &h, mock)
		})
	}
}
