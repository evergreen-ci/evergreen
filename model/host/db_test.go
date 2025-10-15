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
	"go.mongodb.org/mongo-driver/mongo"
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
	assert.NoError(t, db.InsertMany(t.Context(), Collection, h1, h2, h3, h4))

	v1 := Volume{
		ID:        "v1",
		CreatedBy: "me",
	}
	v2 := Volume{
		ID:        "v2",
		CreatedBy: "NOT me",
	}
	assert.NoError(t, db.InsertMany(t.Context(), VolumesCollection, v1, v2))

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

	volumes, err := FindVolumesByUser(t.Context(), "me")
	assert.NoError(t, err)
	assert.Empty(t, volumes)

	volumes, err = FindVolumesByUser(t.Context(), "new_me")
	assert.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, "v1", volumes[0].ID)

	volumes, err = FindVolumesByUser(t.Context(), "NOT me")
	assert.NoError(t, err)
	require.Len(t, volumes, 1)
	assert.Equal(t, "v2", volumes[0].ID)
}

func TestFindUnexpirableRunning(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"ReturnsUnexpirableRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning(ctx)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotReturnExpirableHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = false
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnNonRunningHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.Status = evergreen.HostStopped
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"DoesNotReturnEvergreenOwnedHosts": func(ctx context.Context, t *testing.T, h *Host) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert(ctx))
			hosts, err := FindUnexpirableRunning(ctx)
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
				StartedBy:   "a_task_running_host_create",
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
				{
					Id:     "h4",
					Status: evergreen.HostStarting,
					Distro: distro.Distro{
						Provider:             evergreen.ProviderNameEc2Fleet,
						ProviderAccount:      "account",
						ProviderSettingsList: []*birch.Document{birch.NewDocument()},
					},
				},
			}
			for _, h := range hosts {
				require.NoError(t, h.Insert(ctx))
			}

			hostsByClient, err := FindStartingHostsByClient(ctx, 100)
			assert.NoError(t, err)
			assert.Len(t, hostsByClient, 4)
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
				}:
					require.Len(t, foundHosts, 1)
					compareHosts(t, hosts[1], foundHosts[0])
				case ClientOptions{
					Provider: evergreen.ProviderNameDocker,
				}:
					require.Len(t, foundHosts, 2)
					compareHosts(t, hosts[2], foundHosts[0])
					compareHosts(t, hosts[3], foundHosts[1])
				case ClientOptions{
					Provider: evergreen.ProviderNameEc2Fleet,
					Account:  "account",
				}:
					require.Len(t, foundHosts, 1)
					compareHosts(t, hosts[4], foundHosts[0])
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
			assert.Len(t, hostsByClient, 1)
			require.Equal(t, ClientOptions{Provider: evergreen.ProviderNameEc2Fleet}, hostsByClient[0].Options)
			require.Len(t, hostsByClient[0].Hosts, 1)
			compareHosts(t, hosts[1], hostsByClient[0].Hosts[0])
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

func setupDistroIdStatusIndex(t *testing.T) {
	assert.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{
		Keys: DistroIdStatusIndex,
	}))
}
func TestCountHostsCanRunTasks(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	setupDistroIdStatusIndex(t)
	d1 := distro.Distro{
		Id: "d1",
	}
	h1 := Host{
		Id:        "h1",
		Distro:    d1,
		Status:    evergreen.HostRunning,
		StartedBy: evergreen.User,
	}
	h2 := Host{
		Id:        "h2",
		Distro:    d1,
		Status:    evergreen.HostTerminated,
		StartedBy: evergreen.User,
	}
	h3 := Host{
		Id:        "h3",
		Distro:    d1,
		Status:    evergreen.HostStopping,
		StartedBy: evergreen.User,
	}
	h4 := Host{
		Id:        "h4",
		Distro:    d1,
		Status:    evergreen.HostRunning,
		StartedBy: "testuser",
	}
	h5 := Host{
		Id:        "h5",
		Distro:    d1,
		Status:    evergreen.HostBuilding,
		StartedBy: evergreen.User,
	}
	h6 := Host{
		Id:        "h6",
		Distro:    d1,
		Status:    evergreen.HostProvisionFailed,
		StartedBy: evergreen.User,
	}
	h7 := Host{
		Id:        "h7",
		Distro:    d1,
		Status:    evergreen.HostStopped,
		StartedBy: evergreen.User,
	}
	h8 := Host{
		Id:        "h8",
		Distro:    d1,
		Status:    evergreen.HostProvisioning,
		StartedBy: evergreen.User,
	}
	h9 := Host{
		Id: "h9",
		Distro: distro.Distro{
			Id: "d1",
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodUserData,
			},
		},
		Status:    evergreen.HostStarting,
		StartedBy: evergreen.User,
	}
	h10 := Host{
		Id: "h10",
		Distro: distro.Distro{
			Id: "d1",
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodSSH,
			},
		},
		Status:    evergreen.HostStarting,
		StartedBy: evergreen.User,
	}

	assert.NoError(t, db.InsertMany(t.Context(), Collection, h1, h2, h3, h4, h5, h6, h7, h8, h9, h10))

	ctx := context.TODO()
	count, err := CountHostsCanRunTasks(ctx, "d1")
	require.NoError(t, err)
	assert.Equal(t, 2, count)
}

func TestFindByTemporaryExemptionsExpiringBetween(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"ReturnsHostWithExpiringTemporaryExemption": func(ctx context.Context, t *testing.T, h *Host) {
			now := time.Now()
			h.SleepSchedule.TemporarilyExemptUntil = now
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByTemporaryExemptionsExpiringBetween(ctx, now.Add(-time.Minute), now.Add(time.Minute))
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"ReturnsMultipleHostsWithExpiringTemporaryExemptions": func(ctx context.Context, t *testing.T, h *Host) {
			now := time.Now()
			h.SleepSchedule.TemporarilyExemptUntil = now
			require.NoError(t, h.Insert(ctx))
			h2 := *h
			h2.Id = "h2"
			require.NoError(t, h2.Insert(ctx))

			hosts, err := FindByTemporaryExemptionsExpiringBetween(ctx, now.Add(-time.Minute), now.Add(time.Minute))
			require.NoError(t, err)
			require.Len(t, hosts, 2)
		},
		"ExcludesHostNotWithinExpirationBounds": func(ctx context.Context, t *testing.T, h *Host) {
			now := time.Now()
			h.SleepSchedule.TemporarilyExemptUntil = now
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByTemporaryExemptionsExpiringBetween(ctx, now.Add(-2*time.Minute), now.Add(-time.Minute))
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"ExcludesTerminatedHosts": func(ctx context.Context, t *testing.T, h *Host) {
			now := time.Now()
			h.Status = evergreen.HostTerminated
			h.SleepSchedule.TemporarilyExemptUntil = now
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByTemporaryExemptionsExpiringBetween(ctx, now.Add(-time.Minute), now.Add(time.Minute))
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
				NoExpiration: true,
				StartedBy:    "user",
				Status:       evergreen.HostRunning,
				SleepSchedule: SleepScheduleInfo{
					WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
					TimeZone:         "America/New_York",
				},
			}
			tCase(ctx, t, &h)
		})
	}
}

func TestByUnterminatedSpawnHostsWithInstanceTypes(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, db.ClearCollections(Collection))

	// Create test hosts covering all scenarios
	hosts := []Host{
		// User spawn hosts - should be included in both queries
		{
			Id:           "user-spawn-m5-large",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostRunning,
			InstanceType: "m5.large",
		},
		{
			Id:           "user-spawn-m5-xlarge",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostStopped,
			InstanceType: "m5.xlarge",
		},
		{
			Id:           "user-spawn-c5-large",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostRunning,
			InstanceType: "c5.large",
		},
		// Task host - should be excluded from both queries
		{
			Id:           "task-host",
			UserHost:     false,
			StartedBy:    evergreen.User,
			Status:       evergreen.HostRunning,
			InstanceType: "m5.large",
		},
		// Terminated user spawn host - should be excluded from both queries
		{
			Id:           "user-spawn-terminated",
			UserHost:     true,
			StartedBy:    "test-user",
			Status:       evergreen.HostTerminated,
			InstanceType: "m5.large",
		},
		// User spawn host with no instance type - included in ByUnterminatedSpawnHosts, excluded from WithInstanceTypes
		{
			Id:        "user-spawn-no-instance-type",
			UserHost:  true,
			StartedBy: "test-user",
			Status:    evergreen.HostRunning,
		},
		// Host with UserHost=true but StartedBy = evergreen.User - should be excluded from both
		{
			Id:           "weird-host",
			UserHost:     true,
			StartedBy:    evergreen.User,
			Status:       evergreen.HostRunning,
			InstanceType: "m5.large",
		},
	}

	for _, h := range hosts {
		require.NoError(t, h.Insert(ctx))
	}

	// Test ByUnterminatedSpawnHostsWithInstanceTypes with m5.large
	foundHosts, err := Find(ctx, ByUnterminatedSpawnHostsWithInstanceTypes([]string{"m5.large"}))
	require.NoError(t, err)
	require.Len(t, foundHosts, 1)
	assert.Equal(t, "user-spawn-m5-large", foundHosts[0].Id)

	// Test ByUnterminatedSpawnHostsWithInstanceTypes with multiple instance types
	foundHosts, err = Find(ctx, ByUnterminatedSpawnHostsWithInstanceTypes([]string{"m5.large", "m5.xlarge"}))
	require.NoError(t, err)
	require.Len(t, foundHosts, 2)
	expectedIds := []string{"user-spawn-m5-large", "user-spawn-m5-xlarge"}
	for _, h := range foundHosts {
		assert.Contains(t, expectedIds, h.Id)
	}

	// Test ByUnterminatedSpawnHostsWithInstanceTypes with non-existent instance type
	foundHosts, err = Find(ctx, ByUnterminatedSpawnHostsWithInstanceTypes([]string{"non-existent"}))
	require.NoError(t, err)
	assert.Empty(t, foundHosts)

	// Test ByUnterminatedSpawnHostsWithInstanceTypes with empty list
	foundHosts, err = Find(ctx, ByUnterminatedSpawnHostsWithInstanceTypes([]string{}))
	require.NoError(t, err)
	assert.Len(t, foundHosts, 4)
	for _, h := range foundHosts {
		assert.Contains(t, []string{"user-spawn-m5-large", "user-spawn-m5-xlarge", "user-spawn-c5-large", "user-spawn-no-instance-type"}, h.Id)
	}
}
