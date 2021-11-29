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
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func init() {
	reporting.QuietMode()

	if !utility.StringSliceContains(evergreen.ProviderSpawnable, evergreen.ProviderNameMock) {
		evergreen.ProviderSpawnable = append(evergreen.ProviderSpawnable, evergreen.ProviderNameMock)
	}
}

func setupHostTerminationQueryIndex(t *testing.T) {
	require.NoError(t, db.EnsureIndex(host.Collection, mongo.IndexModel{
		Keys: host.StatusIndex,
	}))
}

func TestHostTerminationJob(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host){
		"TerminatesRunningHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert())
			mcp.Set(h.Id, cloud.MockInstance{
				IsUp:   true,
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, true, "some termination message")
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)

			events, err := event.Find(event.AllLogCollection, event.MostRecentHostEvents(h.Id, "", 50))
			require.NoError(t, err)
			require.NotEmpty(t, events)
			data, ok := events[0].Data.(*event.HostEventData)
			require.True(t, ok)
			assert.Equal(t, "some termination message", data.Logs)

			cloudHost := mcp.Get(h.Id)
			require.NotZero(t, cloudHost)
			assert.Equal(t, cloud.StatusTerminated, cloudHost.Status)
		},
		"NoopsForStaticHosts": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Provider = evergreen.ProviderNameStatic
			h.Distro.Provider = evergreen.ProviderNameStatic
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, true, "foo")
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOne(host.ById(h.Id))
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"FailsWithNonexistentDBHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			j := NewHostTerminationJob(env, h, true, "foo")
			terminationJob, ok := j.(*hostTerminationJob)
			require.True(t, ok)
			terminationJob.host = nil

			j.Run(ctx)
			assert.Error(t, j.Error())
		},
		"ReterminatesCloudHostIfAlreadyMarkedTerminated": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert())
			mcp.Set(h.Id, cloud.MockInstance{
				IsUp:   true,
				Status: cloud.StatusRunning,
			})

			j := NewHostTerminationJob(env, h, true, "foo")
			j.Run(ctx)
			require.NoError(t, j.Error())
		},
		"TerminatesDBHostWithoutCloudHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, true, "foo")
			j.Run(ctx)
			require.Error(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.NotEqual(t, evergreen.HostRunning, dbHost.Status)
		},
		"TerminatesUninitializedIntentHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostUninitialized
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, true, "foo")
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
		"TerminatesBuildingFailedIntentHost": func(ctx context.Context, t *testing.T, env *mock.Environment, mcp cloud.MockProvider, h *host.Host) {
			h.Status = evergreen.HostBuildingFailed
			require.NoError(t, h.Insert())

			j := NewHostTerminationJob(env, h, true, "foo")
			j.Run(ctx)
			require.NoError(t, j.Error())

			dbHost, err := host.FindOneId(h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(host.Collection, event.AllLogCollection), "error clearing host collection")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			// test that trying to terminate a host that does not exist is handled gracecfully
			h := &host.Host{
				Id:          "i-12345",
				Status:      evergreen.HostRunning,
				Distro:      distro.Distro{Provider: evergreen.ProviderNameMock},
				Provider:    evergreen.ProviderNameMock,
				Provisioned: true,
			}

			tCase(ctx, t, env, cloud.GetMockProvider(), h)
		})
	}
}

////////////////////////////////////////////////////////////////////////
//
// Legacy Tests

// Most of these actually test the query "FindHostsToTerminate" which
// is used by the cron rather than actually testing termination
// itself.
//
// At some point these can/should move to the model package.

func TestFlaggingDecommissionedHosts(t *testing.T) {
	Convey("When flagging decommissioned hosts", t, func() {

		Convey("only hosts in the database who are marked decommissioned"+
			" should be returned", func() {

			require.NoError(t, db.DropCollections(host.Collection), "dropping hosts collection")
			defer func() {
				assert.NoError(t, db.DropCollections(host.Collection))
			}()
			setupHostTerminationQueryIndex(t)

			// insert hosts with different statuses

			host1 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h1",
				Status:   evergreen.HostRunning,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			host2 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h2",
				Status:   evergreen.HostTerminated,
			}
			require.NoError(t, host2.Insert(), "error inserting host")

			host3 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h3",
				Status:   evergreen.HostDecommissioned,
			}
			require.NoError(t, host3.Insert(), "error inserting host")

			host4 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h4",
				Status:   evergreen.HostDecommissioned,
			}
			require.NoError(t, host4.Insert(), "error inserting host")

			host5 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h5",
				Status:   evergreen.HostQuarantined,
			}
			require.NoError(t, host5.Insert(), "error inserting host")

			// flag the decommissioned hosts - there should be 2 of them
			decommissioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(decommissioned), ShouldEqual, 2)
			var ids []string
			for _, h := range decommissioned {
				ids = append(ids, h.Id)
			}
			So(utility.StringSliceContains(ids, host3.Id), ShouldBeTrue)
			So(utility.StringSliceContains(ids, host4.Id), ShouldBeTrue)
		})
	})
}

func TestFlaggingUnprovisionedHosts(t *testing.T) {
	Convey("When flagging unprovisioned hosts to be terminated", t, func() {

		require.NoError(t, db.DropCollections(host.Collection), "dropping hosts collection")
		defer func() {
			assert.NoError(t, db.DropCollections(host.Collection))
		}()
		setupHostTerminationQueryIndex(t)

		Convey("hosts that have not hit the provisioning limit should"+
			" be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-time.Minute * 10),
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already terminated should be ignored", func() {
			host1 := &host.Host{
				Id:           "h1",
				Provider:     evergreen.ProviderNameMock,
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Status:       evergreen.HostTerminated,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that are already provisioned should be ignored", func() {
			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Provisioned:  true,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that have hit the provisioning limit should be"+
			" flagged", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Provisioned:  false,
				Status:       evergreen.HostStarting,
				Provider:     evergreen.ProviderNameMock,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h1")
		})
		Convey("hosts that have failed to build should be flagged for termination", func() {
			h := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Status:       evergreen.HostBuildingFailed,
				Provider:     evergreen.ProviderNameMock,
			}
			require.NoError(t, h.Insert())

			found, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
			So(found[0].Id, ShouldEqual, "h1")
		})

		Convey("user data hosts should be ignored if they are running a task or have run a task and recently communicated", func() {
			h1 := &host.Host{
				Id:          "h1",
				Status:      evergreen.HostStarting,
				Provisioned: false,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodUserData,
					},
				},
				LastCommunicationTime: time.Now(),
				RunningTask:           "running_task",
				StartedBy:             evergreen.User,
			}
			require.NoError(t, h1.Insert())
			hosts, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
		Convey("user data hosts that have run tasks before provisioning should be returned if they haven't communicated recently", func() {
			h1 := &host.Host{
				Id:          "h1",
				Status:      evergreen.HostStarting,
				Provisioned: false,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodUserData,
					},
				},
				LastTask:     "last_task",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Provider:     evergreen.ProviderNameEc2Fleet,
			}
			require.NoError(t, h1.Insert())
			hosts, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, h1.Id)
		})
	})
}

func TestFlaggingProvisioningFailedHosts(t *testing.T) {
	Convey("When flagging hosts whose provisioning failed", t, func() {

		require.NoError(t, db.DropCollections(host.Collection), "dropping hosts collection")
		defer func() {
			assert.NoError(t, db.DropCollections(host.Collection))
		}()
		setupHostTerminationQueryIndex(t)

		Convey("only hosts whose provisioning failed should be"+
			" picked up", func() {

			host1 := &host.Host{
				Id:       "h1",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostRunning,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			host2 := &host.Host{
				Id:       "h2",
				Status:   evergreen.HostUninitialized,
				Provider: evergreen.ProviderNameMock,
			}
			require.NoError(t, host2.Insert(), "error inserting host")

			host3 := &host.Host{
				Id:       "h3",
				Status:   evergreen.HostProvisionFailed,
				Provider: evergreen.ProviderNameMock,
			}
			require.NoError(t, host3.Insert(), "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h3")

		})

	})
}

func TestFlaggingExpiredHosts(t *testing.T) {
	Convey("When flagging expired hosts to be terminated", t, func() {

		require.NoError(t, db.DropCollections(host.Collection), "dropping hosts collection")
		defer func() {
			assert.NoError(t, db.DropCollections(host.Collection))
		}()
		setupHostTerminationQueryIndex(t)

		Convey("hosts started by the default user should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:          "h1",
				Status:      evergreen.HostRunning,
				StartedBy:   evergreen.User,
				Provider:    evergreen.ProviderNameMock,
				Provisioned: true,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			expired, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts that are terminated or quarantined should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:       "h1",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostQuarantined,
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			host2 := &host.Host{
				Id:       "h2",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostTerminated,
			}
			require.NoError(t, host2.Insert(), "error inserting host")

			expired, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 0)

		})

		Convey("hosts should be returned if their expiration threshold has"+
			" been reached", func() {

			// not expired
			host1 := &host.Host{
				Id:             "h1",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(time.Minute * 10),
			}
			require.NoError(t, host1.Insert(), "error inserting host")

			// expired
			host2 := &host.Host{
				Id:             "h2",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-time.Minute * 10),
			}
			require.NoError(t, host2.Insert(), "error inserting host")

			expired, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 1)
			So(expired[0].Id, ShouldEqual, "h2")
		})

	})
}
