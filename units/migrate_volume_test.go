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
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVolumeMigrateJob(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx = testutil.TestSpan(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions){
		"VolumeMigratesToNewHost": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert(ctx))
			require.NoError(t, h.Insert(ctx))

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

			volume, err := host.FindVolumeByID(ctx, v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.NotEqual(t, volume.Host, h.Id)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostStopped, initialHost.Status)
			assert.Equal(t, "", initialHost.HomeVolumeID)
			assert.Empty(t, initialHost.Volumes)
			assert.False(t, initialHost.NoExpiration)
			assert.WithinDuration(t, initialHost.ExpirationTime, time.Now(), (time.Hour*24)+time.Second)

			foundHosts, err := host.Find(ctx, host.IsUninitialized)
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
			require.NoError(t, v.Insert(ctx))
			require.NoError(t, h.Insert(ctx))

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			j.UpdateRetryInfo(amboy.JobRetryOptions{
				Retryable: utility.FalsePtr(),
			})
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			var foundFinishedVolumeMigrationJob bool
			// The spawn host start job should fail but may also retry, so wait
			// until the spawn host stop job finishes once (with an error).
			var foundFinishedSpawnHostStopJob bool
			for {
				require.NoError(t, ctx.Err(), "context timed out looking for finished jobs in the queue")
				for ji := range env.RemoteQueue().JobInfo(ctx) {
					if ji.ID == j.ID() && ji.Status.Completed {
						foundFinishedVolumeMigrationJob = true
						assert.NotZero(t, ji.Status.ErrorCount, "volume migration job should have errored")
					}
					if ji.Type.Name == spawnhostStopName && ji.Status.Completed {
						foundFinishedSpawnHostStopJob = true
						assert.NotZero(t, ji.Status.ErrorCount, "spawn host stop job should have errored")
					}
				}
				if foundFinishedSpawnHostStopJob && foundFinishedVolumeMigrationJob {
					break
				}
			}
			assert.True(t, foundFinishedSpawnHostStopJob, "should have found a finished spawn host stop job in the queue")
			assert.True(t, foundFinishedVolumeMigrationJob, "should have found a finished volume migration job in the queue")

			require.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "not yet stopped")

			// Verify volume remains attached to initial host
			volume, err := host.FindVolumeByID(ctx, v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, h.Id, volume.Host)
			assert.False(t, volume.Migrating)

			// And that host is still running
			initialHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, initialHost.Status)
			assert.Equal(t, initialHost.HomeVolumeID, volume.ID)

			events, err := event.FindAllByResourceID(t.Context(), h.Id)
			assert.NoError(t, err)
			require.Len(t, events, 1)
			assert.Equal(t, event.EventVolumeMigrationFailed, events[0].EventType)
		},
		"NewHostFailsToStart": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Invalid public key will prevent new host from spinning up
			spawnOptions.PublicKey = ""
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert(ctx))
			require.NoError(t, h.Insert(ctx))

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
			volume, err := host.FindVolumeByID(ctx, v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.Equal(t, "", volume.Host)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostStopped, initialHost.Status)
			assert.Equal(t, "", initialHost.HomeVolumeID)

			events, err := event.FindAllByResourceID(t.Context(), h.Id)
			assert.NoError(t, err)
			assert.Len(t, events, 3)
		},
		"DetachedVolumeMigrates": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			// Spoof documents after a virtual workstation is terminated.
			// The host document still has its HomeVolumeID set to the volume that was attached while it was running.
			h.Status = evergreen.HostTerminated
			v.Host = ""
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert(ctx))
			require.NoError(t, h.Insert(ctx))

			j := NewVolumeMigrationJob(env, v.ID, spawnOptions, "123")
			require.NoError(t, env.RemoteQueue().Start(ctx))
			require.NoError(t, env.RemoteQueue().Put(ctx, j))

			// First attempt completes with no error since there's no wait for initial host to stop
			require.True(t, amboy.WaitInterval(ctx, env.RemoteQueue(), 1000*time.Millisecond))
			assert.NoError(t, j.Error())

			volume, err := host.FindVolumeByID(ctx, v.ID)
			assert.NoError(t, err)
			assert.NotNil(t, volume)
			assert.NotEqual(t, volume.Host, h.Id)
			assert.False(t, volume.Migrating)

			initialHost, err := host.FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Equal(t, evergreen.HostTerminated, initialHost.Status)
			assert.Equal(t, "v0", initialHost.HomeVolumeID)

			foundHosts, err := host.Find(ctx, host.IsUninitialized)
			assert.NoError(t, err)
			assert.Len(t, foundHosts, 1)
			assert.Equal(t, foundHosts[0].HomeVolumeID, v.ID)
		},
		"NonexistentVolumeFailsGracefully": func(ctx context.Context, t *testing.T, env *mock.Environment, h *host.Host, v *host.Volume, d *distro.Distro, spawnOptions cloud.SpawnOptions) {
			require.NoError(t, d.Insert(ctx))
			require.NoError(t, v.Insert(ctx))
			require.NoError(t, h.Insert(ctx))

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
			assert.Equal(t, 1, j.RetryInfo().GetRemainingAttempts())

			// Second retry fails identically
			j, ok := env.RemoteQueue().Get(ctx, j.ID())
			assert.True(t, ok)
			assert.Error(t, j.Error())
			assert.Contains(t, j.Error().Error(), "volume 'foo' not found")
			assert.Equal(t, 0, j.RetryInfo().GetRemainingAttempts())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(host.Collection, host.VolumesCollection, event.EventCollection, distro.Collection))
			tctx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			tctx = testutil.TestSpan(tctx, t)

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

			const username = "username"
			h := &host.Host{
				Id:           "h0",
				UserHost:     true,
				StartedBy:    username,
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
				UserName:  username,
				DistroId:  d.Id,
				PublicKey: "ssh-rsa YWJjZDEyMzQK",
				Region:    evergreen.DefaultEC2Region,
			}

			testCase(tctx, t, env, h, v, d, spawnOptions)

		})

	}
}
