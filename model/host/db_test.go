package host

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsolidateHostsForUser(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, VolumesCollection))
	h1 := Host{
		Id:        "h1",
		StartedBy: "me",
		Status:    evergreen.HostRunning,
	}
	h2 := Host{
		Id:        "h2",
		StartedBy: "me",
		Status:    evergreen.HostTerminated,
	}
	h3 := Host{
		Id:        "h3",
		StartedBy: "me",
		Status:    evergreen.HostStopped,
	}
	h4 := Host{
		Id:        "h4",
		StartedBy: "NOT me",
		Status:    evergreen.HostRunning,
	}
	assert.NoError(t, db.InsertMany(Collection, h1, h2, h3, h4))

	v1 := Volume{
		ID:        "v1",
		CreatedBy: "me",
	}
	v2 := Volume{
		ID:        "v2",
		CreatedBy: "NOT me",
	}
	assert.NoError(t, db.InsertMany(VolumesCollection, v1, v2))

	ctx := context.TODO()
	assert.NoError(t, ConsolidateHostsForUser(ctx, "me", "new_me"))

	hostFromDB, err := FindOneId(ctx, "h1")
	assert.NoError(t, err)
	assert.Equal(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h2")
	assert.NoError(t, err)
	assert.NotEqual(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h3")
	assert.NoError(t, err)
	assert.Equal(t, "new_me", hostFromDB.StartedBy)

	hostFromDB, err = FindOneId(ctx, "h4")
	assert.NoError(t, err)
	assert.NotEqual(t, "new_me", hostFromDB.StartedBy)

	volumes, err := FindVolumesByUser("me")
	assert.NoError(t, err)
	assert.Len(t, volumes, 0)

	volumes, err = FindVolumesByUser("new_me")
	assert.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, volumes[0].ID, "v1")

	volumes, err = FindVolumesByUser("NOT me")
	assert.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, volumes[0].ID, "v2")
}

func TestFindUnexpirableRunning(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"ReturnsUnexpirableRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotReturnExpirableHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = false
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnNonRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.Status = evergreen.HostStopped
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnEvergreenOwnedHosts": func(ctx context.Context, t *testing.T, h *Host) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			h := Host{
				Id:           "host_id",
				Status:       evergreen.HostRunning,
				StartedBy:    "myself",
				NoExpiration: true,
			}
			tCase(ctx, t, &h)
		})
	}
}

func TestFindStartingHostsByClient(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	compareHosts := func(t *testing.T, host1, host2 Host) {
		assert.Equal(t, host1.Id, host2.Id)
		assert.Equal(t, host1.Status, host2.Status)
		assert.Equal(t, host1.Distro.Provider, host2.Distro.Provider)
		assert.Equal(t, host1.Distro.ProviderSettingsList[0].ExportMap(), host2.Distro.ProviderSettingsList[0].ExportMap())
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"ReturnsStartingTaskHost": func(ctx context.Context, t *testing.T) {
			h := Host{
				Id: "host_id",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:      evergreen.HostStarting,
				Provisioned: false,
				StartedBy:   evergreen.User,
			}
			require.NoError(t, h.Insert(ctx))

			hostsByClient, err := FindStartingHostsByClient(ctx, 1)
			require.NoError(t, err)
			require.Len(t, hostsByClient, 1)
			require.Len(t, hostsByClient[0].Hosts, 1)
			assert.Equal(t, ClientOptions{Provider: evergreen.ProviderNameEc2Fleet}, hostsByClient[0].Options)
			compareHosts(t, hostsByClient[0].Hosts[0], h)
		},
		"IgnoresProvisionedHost": func(ctx context.Context, t *testing.T) {
			h := Host{
				Id: "host_id",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:      evergreen.HostStarting,
				Provisioned: true,
				StartedBy:   evergreen.User,
			}
			require.NoError(t, h.Insert(ctx))

			hostsByClient, err := FindStartingHostsByClient(ctx, 1)
			require.NoError(t, err)
			assert.Empty(t, hostsByClient)
		},
		"IgnoresNonStartingHost": func(ctx context.Context, t *testing.T) {
			h := Host{
				Id: "host_id",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:    evergreen.HostProvisioning,
				StartedBy: evergreen.User,
			}
			require.NoError(t, h.Insert(ctx))

			hostsByClient, err := FindStartingHostsByClient(ctx, 1)
			require.NoError(t, err)
			assert.Empty(t, hostsByClient)
		},
		"ReturnsStartingSpawnHost": func(ctx context.Context, t *testing.T) {
			h := Host{
				Id: "host_id",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:    evergreen.HostStarting,
				StartedBy: "myself",
			}
			require.NoError(t, h.Insert(ctx))

			hostsByClient, err := FindStartingHostsByClient(ctx, 1)
			require.NoError(t, err)
			require.Len(t, hostsByClient, 1)
			assert.Len(t, hostsByClient[0].Hosts, 1)
			assert.Equal(t, ClientOptions{Provider: evergreen.ProviderNameEc2Fleet}, hostsByClient[0].Options)
			compareHosts(t, hostsByClient[0].Hosts[0], h)
		},
		"ReturnsLimitedNumberOfHostsPrioritizedByCreationTime": func(ctx context.Context, t *testing.T) {
			h0 := Host{
				Id: "h0",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:       evergreen.HostStarting,
				StartedBy:    "myself",
				CreationTime: time.Now(),
			}
			h1 := Host{
				Id: "h1",
				Distro: distro.Distro{
					Provider:             evergreen.ProviderNameEc2Fleet,
					ProviderSettingsList: []*birch.Document{birch.NewDocument()},
				},
				Status:       evergreen.HostStarting,
				StartedBy:    "someone_else",
				CreationTime: time.Now().Add(-time.Hour),
			}
			require.NoError(t, h0.Insert(ctx))
			require.NoError(t, h1.Insert(ctx))

			hostsByClient, err := FindStartingHostsByClient(ctx, 1)
			require.NoError(t, err)
			require.Len(t, hostsByClient, 1)
			assert.Equal(t, ClientOptions{Provider: evergreen.ProviderNameEc2Fleet}, hostsByClient[0].Options)
			require.Len(t, hostsByClient[0].Hosts, 1)
			compareHosts(t, hostsByClient[0].Hosts[0], h1)
		},
		"GroupsHostsByClientOptions": func(ctx context.Context, t *testing.T) {
			doc1 := birch.NewDocument(birch.EC.String(awsRegionKey, evergreen.DefaultEC2Region))
			doc2 := birch.NewDocument(
				birch.EC.String(awsRegionKey, "us-west-1"),
				birch.EC.String(awsKeyKey, "key1"),
				birch.EC.String(awsSecretKey, "secret1"),
			)
			hosts := []Host{
				{
					Id:     "h0",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameEc2Fleet,
						ProviderSettingsList: []*birch.Document{doc1},
					},
				},
				{
					Id:     "h1",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameEc2Fleet,
						ProviderSettingsList: []*birch.Document{doc2},
					},
				},
				{
					Id:     "h2",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameDocker,
						ProviderSettingsList: []*birch.Document{birch.NewDocument()},
					},
				},
				{
					Id:     "h3",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameDocker,
						ProviderSettingsList: []*birch.Document{birch.NewDocument()},
					},
				},
			}
			for _, h := range hosts {
				require.NoError(t, h.Insert(ctx))
			}

			hostsByClient, err := FindStartingHostsByClient(ctx, 10)
			assert.NoError(t, err)
			assert.Len(t, hostsByClient, 3)
			for _, hostsByClient := range hostsByClient {
				foundHosts := hostsByClient.Hosts
				clientOptions := hostsByClient.Options
				switch clientOptions {
				case ClientOptions{
					Provider: evergreen.ProviderNameEc2Fleet,
					Region:   evergreen.DefaultEC2Region,
				}:
					require.Len(t, foundHosts, 1)
					compareHosts(t, hosts[0], foundHosts[0])
				case ClientOptions{
					Provider: evergreen.ProviderNameEc2Fleet,
					Region:   "us-west-1",
					Key:      "key1",
					Secret:   "secret1",
				}:
					require.Len(t, foundHosts, 1)
					compareHosts(t, hosts[1], foundHosts[0])
				case ClientOptions{
					Provider: evergreen.ProviderNameDocker,
				}:
					require.Len(t, foundHosts, 2)
					compareHosts(t, hosts[2], foundHosts[0])
					compareHosts(t, hosts[3], foundHosts[1])
				default:
					assert.Fail(t, "unrecognized client options")
				}
			}
		},
		"ReturnsNonTaskHostsBeforeTaskHosts": func(ctx context.Context, t *testing.T) {
			hosts := []Host{
				{
					Id:     "h0",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameEc2Fleet,
						ProviderSettingsList: []*birch.Document{birch.NewDocument()},
					},
					StartedBy:    evergreen.User,
					CreationTime: time.Now().Add(-time.Hour),
				},
				{
					Id:     "h1",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameEc2Fleet,
						ProviderSettingsList: []*birch.Document{birch.NewDocument()},
					},
					StartedBy:    "a_task_running_host_create",
					CreationTime: time.Now(),
				},
			}
			for _, h := range hosts {
				require.NoError(t, h.Insert(ctx))
			}

			hostsByClient, err := FindStartingHostsByClient(ctx, 2)
			assert.NoError(t, err)
			assert.Len(t, hostsByClient, 2)
			require.Equal(t, ClientOptions{Provider: evergreen.ProviderNameEc2Fleet}, hostsByClient[0].Options)
			require.Len(t, hostsByClient[0].Hosts, 1)
			compareHosts(t, hosts[1], hostsByClient[0].Hosts[0])
			require.Len(t, hostsByClient[1].Hosts, 1)
			compareHosts(t, hosts[0], hostsByClient[1].Hosts[0])
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			tCase(ctx, t)
		})
	}
}

func TestFindHostsScheduledToStop(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	now := time.Now()

	for _, tCase := range []struct {
		name          string
		hosts         []Host
		expectedHosts []string
	}{
		{
			name: "ReturnsRunningHostWhoseNextStopHasElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0"},
		},
		{
			name: "ReturnsStoppingHostWhoseNextStopHasElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopping,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0"},
		},
		{
			name: "IgnoresHostWhoseNextStopIsNotAboutToElapse",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(time.Hour),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostThatIsNotStoppable",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostUninitialized,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresPermanentlyExemptHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						PermanentlyExempt: true,
						NextStopTime:      now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresTemporarilyExemptHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						TemporarilyExemptUntil: now.Add(time.Hour),
						NextStopTime:           now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostKeptOff",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						ShouldKeepOff: true,
						NextStopTime:  now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostWithoutASleepSchedule",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresExpirableHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: false,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						ShouldKeepOff: true,
						NextStopTime:  now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "ReturnsMultipleHostsWhoseNextStopHaveElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(-time.Minute),
					},
				},
				{
					Id:           "h1",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(time.Hour),
					},
				},
				{
					Id:           "h2",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopping,
					SleepSchedule: SleepScheduleInfo{
						NextStopTime: now.Add(-3 * time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0", "h2"},
		},
	} {
		t.Run(tCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			for _, h := range tCase.hosts {
				require.NoError(t, h.Insert(ctx))
			}

			oldServiceFlags, err := evergreen.GetServiceFlags(ctx)
			require.NoError(t, err)
			newServiceFlags := *oldServiceFlags
			newServiceFlags.SleepScheduleBetaTestDisabled = true
			require.NoError(t, evergreen.SetServiceFlags(ctx, newServiceFlags))
			defer func() {
				assert.NoError(t, evergreen.SetServiceFlags(ctx, *oldServiceFlags))
			}()

			foundHosts, err := FindHostsScheduledToStop(ctx)
			require.NoError(t, err)
			assert.Len(t, foundHosts, len(tCase.expectedHosts))
			for _, h := range foundHosts {
				assert.Contains(t, tCase.expectedHosts, h.Id)
			}
		})
	}
}

func TestFindHostsScheduledToStart(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	now := time.Now()

	for _, tCase := range []struct {
		name          string
		hosts         []Host
		expectedHosts []string
	}{
		{
			name: "ReturnsStoppedHostsWhoseNextStartHasElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0"},
		},
		{
			name: "ReturnsStoppedHostsWhoseNextStartIsAboutToElapse",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0"},
		},
		{
			name: "ReturnsStoppingHostWhoseNextStartHasElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopping,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0"},
		},
		{
			name: "IgnoresHostWhoseNextStartHasNotElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(time.Hour),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostThatIsNotStartable",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostUninitialized,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresPermanentlyExemptHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						PermanentlyExempt: true,
						NextStartTime:     now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresTemporarilyExemptHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						TemporarilyExemptUntil: now.Add(time.Hour),
						NextStartTime:          now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostKeptOff",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						ShouldKeepOff: true,
						NextStartTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresHostWithoutASleepSchedule",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostRunning,
				},
			},
			expectedHosts: nil,
		},
		{
			name: "IgnoresExpirableHost",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: false,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						ShouldKeepOff: true,
						NextStartTime: now.Add(-time.Minute),
					},
				},
			},
			expectedHosts: nil,
		},
		{
			name: "ReturnsMultipleHostsWhoseNextStartHaveElapsed",
			hosts: []Host{
				{
					Id:           "h0",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(-time.Minute),
					},
				},
				{
					Id:           "h1",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopped,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(time.Hour),
					},
				},
				{
					Id:           "h2",
					StartedBy:    "myself",
					NoExpiration: true,
					Status:       evergreen.HostStopping,
					SleepSchedule: SleepScheduleInfo{
						NextStartTime: now.Add(-3 * time.Minute),
					},
				},
			},
			expectedHosts: []string{"h0", "h2"},
		},
	} {
		t.Run(tCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			for _, h := range tCase.hosts {
				require.NoError(t, h.Insert(ctx))
			}

			oldServiceFlags, err := evergreen.GetServiceFlags(ctx)
			require.NoError(t, err)
			newServiceFlags := *oldServiceFlags
			newServiceFlags.SleepScheduleBetaTestDisabled = true
			require.NoError(t, evergreen.SetServiceFlags(ctx, newServiceFlags))
			defer func() {
				assert.NoError(t, evergreen.SetServiceFlags(ctx, *oldServiceFlags))
			}()

			foundHosts, err := FindHostsScheduledToStart(ctx)
			require.NoError(t, err)
			assert.Len(t, foundHosts, len(tCase.expectedHosts))
			for _, h := range foundHosts {
				assert.Contains(t, tCase.expectedHosts, h.Id)
			}
		})
	}
}

func TestCountRunningStatusHosts(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	d1 := distro.Distro{
		Id: "d1",
	}
	h1 := Host{
		Id:     "h1",
		Distro: d1,
		Status: evergreen.HostRunning,
	}
	h2 := Host{
		Id:     "h2",
		Distro: d1,
		Status: evergreen.HostTerminated,
	}
	h3 := Host{
		Id:     "h3",
		Distro: d1,
		Status: evergreen.HostStopping,
	}
	h4 := Host{
		Id:     "h4",
		Distro: d1,
		Status: evergreen.HostRunning,
	}
	h5 := Host{
		Id:     "h5",
		Distro: d1,
		Status: evergreen.HostBuilding,
	}
	h6 := Host{
		Id:     "h6",
		Distro: d1,
		Status: evergreen.HostProvisionFailed,
	}
	h7 := Host{
		Id:     "h7",
		Distro: d1,
		Status: evergreen.HostStopped,
	}
	h8 := Host{
		Id:     "h8",
		Distro: d1,
		Status: evergreen.HostProvisioning,
	}

	assert.NoError(t, db.InsertMany(Collection, h1, h2, h3, h4, h5, h6, h7, h8))

	ctx := context.TODO()
	count, err := CountRunningStatusHosts(ctx, "d1")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)

}
