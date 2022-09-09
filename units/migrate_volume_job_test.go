package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeMigrateJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sshKeyName, sshKeyValue := "foo", "bar"

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume){
		"NewVolumeMigrationJob": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume) {
			assert.NoError(t, h.Insert())
			assert.NoError(t, v.Insert())

			mock := cloud.GetMockProvider()
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewVolumeMigrationJob(env, "v0", ts)

			require.NoError(t, env.RemoteQueue().Put(ctx, j))
			require.NoError(t, env.RemoteQueue().Start(ctx))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), time.Millisecond))
			assert.True(t, j.RetryInfo().ShouldRetry())

			host0, err := host.FindOne(host.ById("h0"))
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostStopped, host0.Status)
			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 1000*time.Millisecond))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.LegacyEventLogCollection))
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			// Don't use the default local limited size remote queue from the
			// mock env because it does not accept jobs when it's not active.
			rq, err := queue.NewLocalLimitedSizeSerializable(1, 1)
			require.NoError(t, err)
			env.Remote = rq

			env.Settings().Keys = map[string]string{sshKeyName: sshKeyValue}

			h := &host.Host{
				Id:           "h0",
				UserHost:     true,
				Status:       evergreen.HostRunning,
				Provider:     evergreen.ProviderNameMock,
				Distro:       distro.Distro{Provider: evergreen.ProviderNameMock},
				HomeVolumeID: "v0",
				Volumes: []host.VolumeAttachment{
					{
						VolumeID:   "v0",
						DeviceName: "test-device-name",
					},
				},
				InstanceTags: []host.Tag{
					{
						Key:           "key-1",
						Value:         "val-1",
						CanBeModified: true,
					},
				},
			}

			v := &host.Volume{
				ID:               "v0",
				Host:             "h0",
				HomeVolume:       true,
				AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
				CreatedBy:        "test-user",
				Type:             "standard",
				Size:             32,
				Expiration:       time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			}

			testCase(tctx, t, env, h, v)

		})

	}
}
