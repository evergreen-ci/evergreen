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
			require.NoError(t, d.Insert())
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
			j, updatedJ := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, updatedJ)
			assert.NoError(t, j.Error())

			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.NotEqual(t, h.Id, volume.Host)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, initialHost.Status)
			assert.Equal(t, "", initialHost.HomeVolumeID)
			assert.Empty(t, initialHost.Volumes)

			foundHosts, err := host.Find(host.IsUninitialized)
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
			require.NoError(t, d.Insert())
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil: utility.ToTimeDurationPtr(0),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")
			// Second attempt fails to stop initial host
			j, updatedJ := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, updatedJ)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "failed to stop")

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
			assert.Equal(t, h.HomeVolumeID, volume.ID)
		},
		"VolumeFailsToUnmount": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Empty volume ID will prevent the volume from detaching
			v.ID = ""
			require.NoError(t, d.Insert())
			require.NoError(t, v.Insert())
			require.NoError(t, h.Insert())

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				WaitUntil: utility.ToTimeDurationPtr(0),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 100*time.Millisecond))
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")

			j, updatedJ := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, updatedJ)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "detaching volume")

			// Initial host should have restarted with volume attached
			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, h.Id, volume.Host)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostRunning)
			assert.Equal(t, h.HomeVolumeID, "v0")
		},
		"NewHostFailsToStart": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Invalid public key will prevent new host from spinning up
			spawnOptions.PublicKey = ""
			require.NoError(t, d.Insert())
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
			j, updatedJ := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, updatedJ)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "creating new intent host")

			// Initial host should have restarted with volume attached
			volume, err := host.FindVolumeByID(v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, h.Id, volume.Host)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(h.Id)
			assert.NoError(t, err)
			assert.Equal(t, initialHost.Status, evergreen.HostRunning)
			assert.Equal(t, h.HomeVolumeID, volume.ID)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.LegacyEventLogCollection, distro.Collection, dispatcher.Collection))
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
				CreatedBy:        "test-user",
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
