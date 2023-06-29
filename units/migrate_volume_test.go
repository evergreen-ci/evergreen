package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeMigrateJob(t *testing.T) {
	c, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions){
		"VolumeMigratesToNewHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			ts := utility.RoundPartOfMinute(1).Format(TSFormat)
			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, ts)
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil: utility.ToTimeDurationPtr(0),
			})

			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))

			// First attempt fails with an error
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")

			// Second attempt completes no error
			j, ok := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, ok)
			assert.NoError(t, j.Error())

			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.NotEqual(t, volume.Host, h.Id)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostStopped)
			assert.Equal(t, initialHost.HomeVolumeID, "")
			assert.Empty(t, initialHost.Volumes)
			assert.False(t, initialHost.NoExpiration)
			assert.WithinDuration(t, initialHost.ExpirationTime, time.Now(), (time.Hour*24)+time.Second)

			foundHosts, err := host.FindWithContext(ctx, host.IsUninitialized)
			assert.NoError(t, err)
			assert.Len(t, foundHosts, 1)
			assert.Equal(t, foundHosts[0].HomeVolumeID, v.ID)
		},
		"RejectsJobWithSameVolumeID": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			dupeJ := NewVolumeMigrationJob(env, v.ID, spawnOptions, "456")
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))
			require.Error(t, env.RemoteQueue().Put(ctx, dupeJ))
		},
		"InitialHostFailsToStop": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Unsetting the Provider will cause the initial host to fail to stop
			d.Provider = ""
			h.Distro.Provider = ""
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil:   utility.ToTimeDurationPtr(100 * time.Millisecond),
				MaxAttempts: utility.ToIntPtr(3),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")
			// Second attempt fails to stop initial host
			j, ok := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, ok)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")

			// Verify volume remains attached to initial host
			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, h.Id, volume.Host)
			assert.False(t, volume.Migrating)

			// And that host is still running
			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostRunning)
			assert.Equal(t, initialHost.HomeVolumeID, volume.ID)

			events, err := event.FindAllByResourceID(h.Id)
			assert.NoError(t, err)
			assert.Len(t, events, 2)
			assert.Equal(t, events[0].EventType, event.EventHostCreated)
			assert.Equal(t, events[1].EventType, event.EventVolumeMigrationFailed)
		},
		"NewHostFailsToStart": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Invalid public key will prevent new host from spinning up
			spawnOptions.PublicKey = ""
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			// Limit retry attempts, since a failure to spawn a host will cause a retry
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil:   utility.ToTimeDurationPtr(100 * time.Millisecond),
				MaxAttempts: utility.ToIntPtr(3),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 1000*time.Millisecond))
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")

			// Second attempt fails to start new host
			j, ok := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, ok)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "creating new intent host")

			// Should finish with initial host stopped and volume detached
			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, volume.Host, "")
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostStopped)
			assert.Equal(t, initialHost.HomeVolumeID, "")

			events, err := event.FindAllByResourceID(h.Id)
			assert.NoError(t, err)
			assert.Len(t, events, 4)
		},
		"DetachedVolumeMigrates": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Spoof documents after a virtual workstation is terminated.
			// The host document still has its HomeVolumeID set to the volume that was attached while it was running.
			h.Status = evergreen.HostTerminated
			v.Host = ""
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			// First attempt completes with no error since there's no wait for initial host to stop
			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 1000*time.Millisecond))
			assert.NoError(t, j.Error())

			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.NotEqual(t, volume.Host, h.Id)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostTerminated)
			assert.Equal(t, initialHost.HomeVolumeID, "v0")

			foundHosts, err := host.FindWithContext(ctx, host.IsUninitialized)
			assert.NoError(t, err)
			assert.Len(t, foundHosts, 1)
			assert.Equal(t, foundHosts[0].HomeVolumeID, v.ID)
		},
		"NonexistentVolumeFailsGracefully": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, "foo", spawnOptions, "123")
			// Limit retry attempts, since a failure to find a volume will cause a retry
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil:   utility.ToTimeDurationPtr(100 * time.Millisecond),
				MaxAttempts: utility.ToIntPtr(2),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 1000*time.Millisecond))
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "volume 'foo' not found")
			assert.Equal(t, j.RetryInfo().GetRemainingAttempts(), 1)

			// Second retry fails identically
			j, ok := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, ok)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "volume 'foo' not found")
			assert.Equal(t, j.RetryInfo().GetRemainingAttempts(), 0)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.EventCollection, distro.Collection, dispatcher.Collection))
			tctx, cancel := context.WithTimeout(c, 30*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))

			// Don't use the default local limited size remote queue from the
			// mock env because it does not accept jobs when it's not active.
			rq, err := queue.NewLocalLimitedSizeSerializable(1, 8)
			require.NoError(t, err)
			env.Remote = rq

			env.Settings().Providers.AWS.Subnets = []evergreen.Subnet{
				{AZ: "us-east-1a", SubnetID: "56789"},
			}

			originalEnv := evergreen.GetEnvironment()
			evergreen.SetEnvironment(env)
			defer func() {
				evergreen.SetEnvironment(originalEnv)
			}()

			d := &distro.Distro{
				Id:                   "d0",
				Provider:             evergreen.ProviderNameMock,
				ProviderSettingsList: []*birch.Document{birch.NewDocument(birch.EC.String("region", evergreen.DefaultEC2Region))},
				HostAllocatorSettings: distro.HostAllocatorSettings{
					AcceptableHostIdleTime: 4 * time.Minute,
				},
				SpawnAllowed: true,
			}

			h := &host.Host{
				Id:           "h0",
				UserHost:     true,
				StartedBy:    evergreen.User,
				Status:       evergreen.HostRunning,
				Provider:     evergreen.ProviderNameMock,
				Distro:       *d,
				HomeVolumeID: "v0",
				Zone:         evergreen.DefaultEBSAvailabilityZone,
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

			mock := cloud.GetMockProvider()
			mock.Set(h.Id, cloud.MockInstance{
				Status: cloud.StatusRunning,
			})

			v := &host.Volume{
				ID:               "v0",
				Host:             h.Id,
				HomeVolume:       true,
				AvailabilityZone: evergreen.DefaultEBSAvailabilityZone,
				CreatedBy:        evergreen.User,
				Type:             "standard",
				Size:             32,
				Expiration:       time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			}

			spawnOptions := cloud.SpawnOptions{
				DistroId:  d.Id,
				PublicKey: "ssh-rsa YWJjZDEyMzQK",
				Region:    evergreen.DefaultEC2Region,
			}

			testCase(tctx, t, env, h, v, d, spawnOptions)

		})

	}
}
