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
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/assert"
)

func init() {
	reporting.QuietMode()

	if !util.StringSliceContains(evergreen.ProviderSpawnable, evergreen.ProviderNameMock) {
		evergreen.ProviderSpawnable = append(evergreen.ProviderSpawnable, evergreen.ProviderNameMock)
	}
}

func TestTerminateHosts(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestTerminateHosts")
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.Clear(host.Collection), t, "error clearing host collection")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	assert.NoError(env.Configure(ctx, "", nil))
	assert.NoError(env.Local.Start(ctx))

	hostID := "i-12345"
	mcp := cloud.GetMockProvider()
	mcp.Set(hostID, cloud.MockInstance{
		IsUp:   true,
		Status: cloud.StatusRunning,
	})

	// test that trying to terminate a host that does not exist is handled gracecfully
	h := &host.Host{
		Id:          hostID,
		Status:      evergreen.HostRunning,
		Provider:    evergreen.ProviderNameMock,
		Provisioned: true,
	}
	assert.NoError(h.Insert())
	j := NewHostTerminationJob(env, *h, true)
	j.Run(ctx)

	assert.NoError(j.Error())
	dbHost, err := host.FindOne(host.ById(h.Id))
	assert.NoError(err)
	assert.NotNil(dbHost)
	assert.Equal(evergreen.HostTerminated, dbHost.Status)
}

func TestHostCosts(t *testing.T) {
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestHostCosts")
	assert := assert.New(t)
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	testutil.HandleTestingErr(db.ClearCollections(host.Collection, task.Collection), t, "error clearing host collection")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hostID := "i-12345"
	mcp := cloud.GetMockProvider()
	mcp.Set(hostID, cloud.MockInstance{
		IsUp:   true,
		Status: cloud.StatusRunning,
	})
	h := &host.Host{
		Id: hostID,
		Distro: distro.Distro{
			Provider: evergreen.ProviderNameMock,
		},
		Status:      evergreen.HostRunning,
		StartedBy:   "t1",
		Provider:    evergreen.ProviderNameMock,
		Provisioned: true,
		StartTime:   time.Now().Add(-5 * time.Minute),
		SpawnOptions: host.SpawnOptions{
			SpawnedByTask: true,
		},
	}
	assert.NoError(h.Insert())
	t1 := task.Task{
		Id: "t1",
	}
	assert.NoError(t1.Insert())

	j := NewHostTerminationJob(&mock.Environment{}, *h, true)
	j.Run(ctx)
	assert.NoError(j.Error())
	dbHost, err := host.FindOne(host.ById(h.Id))
	assert.NoError(err)
	assert.NotNil(dbHost)
	assert.Equal(evergreen.HostTerminated, dbHost.Status)
	assert.InDelta(5, dbHost.TotalCost, 0.001)
	dbTask, err := task.FindOneId(t1.Id)
	assert.NoError(err)
	assert.InDelta(5, dbTask.SpawnedHostCost, 0.001)
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

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When flagging decommissioned hosts", t, func() {

		Convey("only hosts in the database who are marked decommissioned"+
			" should be returned", func() {

			// reset the db
			testutil.HandleTestingErr(db.ClearCollections(host.Collection),
				t, "error clearing hosts collection")

			// insert hosts with different statuses

			host1 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h1",
				Status:   evergreen.HostRunning,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h2",
				Status:   evergreen.HostTerminated,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h3",
				Status:   evergreen.HostDecommissioned,
			}
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			host4 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h4",
				Status:   evergreen.HostDecommissioned,
			}
			testutil.HandleTestingErr(host4.Insert(), t, "error inserting host")

			host5 := &host.Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h5",
				Status:   evergreen.HostQuarantined,
			}
			testutil.HandleTestingErr(host5.Insert(), t, "error inserting host")

			// flag the decommissioned hosts - there should be 2 of them
			decommissioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(decommissioned), ShouldEqual, 2)
			var ids []string
			for _, h := range decommissioned {
				ids = append(ids, h.Id)
			}
			So(util.StringSliceContains(ids, host3.Id), ShouldBeTrue)
			So(util.StringSliceContains(ids, host4.Id), ShouldBeTrue)
		})
	})
}

func TestFlaggingUnprovisionedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When flagging unprovisioned hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts that have not hit the provisioning limit should"+
			" be ignored", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-time.Minute * 10),
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

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
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

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
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 0)

		})

		Convey("hosts that have hit the provisioning limit should be"+
			" flagged", func() {

			host1 := &host.Host{
				Id:           "h1",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Provisioned:  false,
				Status:       evergreen.HostStarting,
				Provider:     evergreen.ProviderNameMock,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h1")

		})

	})
}

func TestFlaggingProvisioningFailedHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When flagging hosts whose provisioning failed", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("only hosts whose provisioning failed should be"+
			" picked up", func() {

			host1 := &host.Host{
				Id:       "h1",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostRunning,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:       "h2",
				Status:   evergreen.HostUninitialized,
				Provider: evergreen.ProviderNameMock,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			host3 := &host.Host{
				Id:       "h3",
				Status:   evergreen.HostProvisionFailed,
				Provider: evergreen.ProviderNameMock,
			}
			testutil.HandleTestingErr(host3.Insert(), t, "error inserting host")

			unprovisioned, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(unprovisioned), ShouldEqual, 1)
			So(unprovisioned[0].Id, ShouldEqual, "h3")

		})

	})
}

func TestFlaggingExpiredHosts(t *testing.T) {

	testConfig := testutil.TestConfig()

	db.SetGlobalSessionProvider(testConfig.SessionFactory())

	Convey("When flagging expired hosts to be terminated", t, func() {

		// reset the db
		testutil.HandleTestingErr(db.ClearCollections(host.Collection),
			t, "error clearing hosts collection")

		Convey("hosts started by the default user should be filtered"+
			" out", func() {

			host1 := &host.Host{
				Id:          "h1",
				Status:      evergreen.HostRunning,
				StartedBy:   evergreen.User,
				Provider:    evergreen.ProviderNameMock,
				Provisioned: true,
			}
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

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
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			host2 := &host.Host{
				Id:       "h2",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostTerminated,
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

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
			testutil.HandleTestingErr(host1.Insert(), t, "error inserting host")

			// expired
			host2 := &host.Host{
				Id:             "h2",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-time.Minute * 10),
			}
			testutil.HandleTestingErr(host2.Insert(), t, "error inserting host")

			expired, err := host.FindHostsToTerminate()
			So(err, ShouldBeNil)
			So(len(expired), ShouldEqual, 1)
			So(expired[0].Id, ShouldEqual, "h2")
		})
	})
}
