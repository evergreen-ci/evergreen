package host

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func init() {
	testutil.Setup()
}

// IsActive is a query that returns all Evergreen hosts that are working or
// capable of being assigned work to do.
var IsActive = bson.M{
	StartedByKey: evergreen.User,
	StatusKey: bson.M{
		"$nin": []string{
			evergreen.HostTerminated, evergreen.HostDecommissioned,
		},
	},
}

func hostIdInSlice(hosts []Host, id string) bool {
	for _, host := range hosts {
		if host.Id == id {
			return true
		}
	}
	return false
}

func TestGenericHostFinding(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When finding hosts", t, func() {
		require.NoError(t, db.Clear(Collection))

		Convey("when finding one host", func() {
			Convey("the matching host should be returned", func() {
				matchingHost := &Host{
					Id: "matches",
				}
				So(matchingHost.Insert(ctx), ShouldBeNil)

				nonMatchingHost := &Host{
					Id: "nonMatches",
				}
				So(nonMatchingHost.Insert(ctx), ShouldBeNil)

				found, err := FindOne(ctx, ById(matchingHost.Id))
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, matchingHost.Id)

			})
		})

		Convey("when finding multiple hosts", func() {
			dId := fmt.Sprintf("%v.%v", DistroKey, distro.IdKey)

			Convey("the hosts matching the query should be returned", func() {
				matchingHostOne := &Host{
					Id:     "matches",
					Distro: distro.Distro{Id: "d1"},
				}
				So(matchingHostOne.Insert(ctx), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Distro: distro.Distro{Id: "d1"},
				}
				So(matchingHostTwo.Insert(ctx), ShouldBeNil)

				nonMatchingHost := &Host{
					Id:     "nonMatches",
					Distro: distro.Distro{Id: "d2"},
				}
				So(nonMatchingHost.Insert(ctx), ShouldBeNil)

				found, err := Find(ctx, bson.M{dId: "d1"})
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 2)
				So(hostIdInSlice(found, matchingHostOne.Id), ShouldBeTrue)
				So(hostIdInSlice(found, matchingHostTwo.Id), ShouldBeTrue)

			})

			Convey("when querying two hosts for running tasks", func() {
				matchingHost := &Host{Id: "task", Status: evergreen.HostRunning, RunningTask: "t1"}
				So(matchingHost.Insert(ctx), ShouldBeNil)
				nonMatchingHost := &Host{Id: "nope", Status: evergreen.HostRunning}
				So(nonMatchingHost.Insert(ctx), ShouldBeNil)
				Convey("the host with the running task should be returned", func() {
					found, err := Find(ctx, IsRunningTask)
					So(err, ShouldBeNil)
					So(len(found), ShouldEqual, 1)
					So(found[0].Id, ShouldEqual, matchingHost.Id)
				})
			})

			Convey("the specified projection, sort, skip, and limit should be used", func() {
				matchingHostOne := &Host{
					Id:     "matches",
					Host:   "hostOne",
					Distro: distro.Distro{Id: "d1"},
					Tag:    "2",
				}
				So(matchingHostOne.Insert(ctx), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Host:   "hostTwo",
					Distro: distro.Distro{Id: "d1"},
					Tag:    "1",
				}
				So(matchingHostTwo.Insert(ctx), ShouldBeNil)

				matchingHostThree := &Host{
					Id:     "stillMatches",
					Host:   "hostThree",
					Distro: distro.Distro{Id: "d1"},
					Tag:    "3",
				}
				So(matchingHostThree.Insert(ctx), ShouldBeNil)

				// find the hosts, removing the host field from the projection,
				// sorting by tag, skipping one, and limiting to one

				found, err := Find(ctx, bson.M{dId: "d1"}, options.Find().
					SetProjection(bson.M{DNSKey: 0}).
					SetSort(bson.M{TagKey: 1}).
					SetSkip(1).
					SetLimit(1))
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 1)
				So(found[0].Id, ShouldEqual, matchingHostOne.Id)
				So(found[0].Host, ShouldEqual, "") // filtered out in projection
			})
		})
	})
}

func TestFindingHostsWithRunningTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host with no running task that is not terminated", t, func() {
		require.NoError(t, db.Clear(Collection))
		h := Host{
			Id:     "sample_host",
			Status: evergreen.HostRunning,
		}
		So(h.Insert(ctx), ShouldBeNil)
		found, err := Find(ctx, IsRunningTask)
		So(err, ShouldBeNil)
		So(len(found), ShouldEqual, 0)
		Convey("with a host that is terminated with no running task", func() {
			require.NoError(t, db.Clear(Collection))
			h1 := Host{
				Id:     "another",
				Status: evergreen.HostTerminated,
			}
			So(h1.Insert(ctx), ShouldBeNil)
			found, err = Find(ctx, IsRunningTask)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 0)
		})
	})

}

func TestMonitorHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host with no reachability check", t, func() {
		require.NoError(t, db.Clear(Collection))
		now := time.Now()
		h := Host{
			Id:        "sample_host",
			Status:    evergreen.HostRunning,
			StartedBy: evergreen.User,
		}
		So(h.Insert(ctx), ShouldBeNil)
		found, err := Find(ctx, ByNotMonitoredSince(now))
		So(err, ShouldBeNil)
		So(len(found), ShouldEqual, 1)
		Convey("a host that has a running task and no reachability check should not return", func() {
			require.NoError(t, db.Clear(Collection))
			anotherHost := Host{
				Id:          "anotherHost",
				Status:      evergreen.HostRunning,
				StartedBy:   evergreen.User,
				RunningTask: "id",
			}
			So(anotherHost.Insert(ctx), ShouldBeNil)
			found, err := Find(ctx, ByNotMonitoredSince(now))
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 0)
		})
		Convey("a non-user data provisioned host with no reachability check should not return", func() {
			require.NoError(t, db.Clear(Collection))
			h := Host{
				Id:        "id",
				Status:    evergreen.HostStarting,
				StartedBy: evergreen.User,
			}
			So(h.Insert(ctx), ShouldBeNil)
			found, err := Find(ctx, ByNotMonitoredSince(now))
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 0)
		})
		Convey("a user data provisioned host with no reachability check should return", func() {
			require.NoError(t, db.Clear(Collection))
			h := Host{
				Id:          "id",
				Status:      evergreen.HostStarting,
				StartedBy:   evergreen.User,
				Provisioned: true,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodUserData,
					},
				},
			}
			So(h.Insert(ctx), ShouldBeNil)
			found, err := Find(ctx, ByNotMonitoredSince(now))
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 1)
		})
	})
}

func TestUpdatingHostStatus(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {
		require.NoError(t, db.Clear(Collection))

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(ctx), ShouldBeNil)

		Convey("setting the host's status should update both the in-memory"+
			" and database versions of the host", func() {

			So(host.SetStatus(ctx, evergreen.HostRunning, evergreen.User, ""), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
		})

		Convey("if the host is terminated, the status update should fail"+
			" with an error", func() {

			So(host.SetStatus(ctx, evergreen.HostTerminated, evergreen.User, ""), ShouldBeNil)
			So(host.SetStatus(ctx, evergreen.HostRunning, evergreen.User, ""), ShouldNotBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
		})
	})
}

func TestSetStatusAndFields(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, event.EventCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, h *Host){
		"FailsIfHostDoesNotExist": func(t *testing.T, h *Host) {
			assert.Error(t, h.setStatusAndFields(ctx, evergreen.HostDecommissioned, nil, nil, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			assert.Zero(t, dbHost)
		},
		"DoesNotUpdatesAnyFieldsIfStatusIsIdentical": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			currentStatus := h.Status
			newDisplayName := "new-display-name"
			require.NoError(t, h.setStatusAndFields(ctx, currentStatus, nil, bson.M{DisplayNameKey: newDisplayName}, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, currentStatus, dbHost.Status, "host status should not be updated when host status is identical")
			assert.Zero(t, dbHost.DisplayName, "display name should not be updated when host status is identical")
		},
		"UpdatesOnlyStatus": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			newStatus := evergreen.HostQuarantined
			require.NoError(t, h.setStatusAndFields(ctx, newStatus, nil, nil, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, newStatus, dbHost.Status)
		},
		"UpdatesStatusAndAdditionalFields": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			newStatus := evergreen.HostQuarantined
			newDisplayName := "new-display-name"
			require.NoError(t, h.setStatusAndFields(ctx, newStatus, nil, bson.M{DisplayNameKey: newDisplayName}, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, newStatus, dbHost.Status)
			assert.Equal(t, newDisplayName, dbHost.DisplayName)
		},
		"UpdatesStatusAndUnsetsFields": func(t *testing.T, h *Host) {
			h.LastTask = "taskZero"
			h.RunningTask = "taskHero"
			assert.NoError(t, h.Insert(ctx))

			newStatus := evergreen.HostQuarantined
			assert.NoError(t, h.setStatusAndFields(ctx, newStatus, nil, nil, bson.M{LTCTaskKey: 1}, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			assert.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.Equal(t, newStatus, dbHost.Status)
			assert.Equal(t, "taskHero", dbHost.RunningTask)
			assert.Empty(t, dbHost.LastTask)
		},
		"FailsForAlreadyTerminatedHost": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			assert.Error(t, h.setStatusAndFields(ctx, evergreen.HostRunning, nil, nil, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, evergreen.HostTerminated, dbHost.Status, "terminated host should remain terminated")
		},
		"FailsDecommissionIfHostIsNowRunningTaskGroup": func(t *testing.T, h *Host) {
			h.RunningTaskGroup = "my_task_group"
			require.NoError(t, h.Insert(ctx))
			query := bson.M{
				RunningTaskGroupKey: bson.M{"$eq": ""},
			}
			assert.Error(t, h.setStatusAndFields(ctx, evergreen.HostDecommissioned, query, nil, nil, evergreen.User, ""))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotNil(t, dbHost)
			assert.NotEqual(t, evergreen.HostDecommissioned, h.Status)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, event.EventCollection))
			h := Host{
				Id:     "host",
				Status: evergreen.HostRunning,
			}
			tCase(t, &h)
		})
	}
}

func TestDecommissionHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id:          "myHost",
		RunningTask: "runningTask",
		Status:      evergreen.HostRunning,
	}
	assert.NoError(t, h.Insert(ctx))

	assert.NoError(t, h.SetDecommissioned(ctx, "user", false, "because I said so"))

	// Updating shouldn't work because we have a running task.
	hostFromDb, err := FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	require.NotNil(t, hostFromDb)
	assert.NotEqual(t, evergreen.HostDecommissioned, hostFromDb.Status)
	assert.NotEqual(t, evergreen.HostDecommissioned, h.Status)

	assert.NoError(t, h.SetDecommissioned(ctx, "user", true, "counting to three"))

	// Updating should work because we set terminate if busy.
	hostFromDb, err = FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	require.NotNil(t, hostFromDb)
	assert.Equal(t, evergreen.HostDecommissioned, hostFromDb.Status)
	assert.Equal(t, evergreen.HostDecommissioned, h.Status)
}

func TestSetStopped(t *testing.T) {
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"StopsHost": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			assert.NoError(t, h.SetStopped(ctx, false, ""))
			assert.Equal(t, evergreen.HostStopped, h.Status)
			assert.Empty(t, h.Host)
			assert.True(t, utility.IsZeroTime(h.StartTime))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)
			assert.Empty(t, dbHost.Host)
			assert.True(t, utility.IsZeroTime(dbHost.StartTime))
			assert.False(t, h.SleepSchedule.ShouldKeepOff)
			assert.NotZero(t, h.SleepSchedule.NextStopTime, "next stop time should remain set on the host")
			assert.NotZero(t, h.SleepSchedule.NextStartTime, "next start time should remain set on the host")
		},
		"SetsShouldKeepOffAndClearsNextSleepScheduleTimes": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			assert.NoError(t, h.SetStopped(ctx, true, ""))
			assert.Equal(t, evergreen.HostStopped, h.Status)
			assert.Empty(t, h.Host)
			assert.True(t, utility.IsZeroTime(h.StartTime))
			assert.True(t, h.SleepSchedule.ShouldKeepOff)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStopped, dbHost.Status)
			assert.Empty(t, dbHost.Host)
			assert.True(t, utility.IsZeroTime(dbHost.StartTime))
			assert.True(t, dbHost.SleepSchedule.ShouldKeepOff)
			assert.Zero(t, h.SleepSchedule.NextStopTime, "next stop time should be cleared on a host being kept off")
			assert.Zero(t, h.SleepSchedule.NextStartTime, "next start time should be cleared on a host being kept off")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.Clear(Collection))
			h := &Host{
				Id:        "host",
				Status:    evergreen.HostRunning,
				StartTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
				Host:      "host.mongodb.com",
				SleepSchedule: SleepScheduleInfo{
					NextStartTime: time.Now(),
					NextStopTime:  time.Now(),
				},
			}
			tCase(ctx, t, h)
		})
	}
}

func TestSetHostTerminated(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(ctx), ShouldBeNil)

		Convey("setting the host as terminated should set the status and the"+
			" termination time in both the in-memory and database copies of"+
			" the host", func() {

			So(host.Terminate(ctx, evergreen.User, ""), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

		})

	})
}

func TestHostSetDNSName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(ctx), ShouldBeNil)

		Convey("setting the hostname should update both the in-memory and"+
			" database copies of the host", func() {

			So(host.SetDNSName(ctx, "hostname"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")
			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			// if the host is already updated, no new updates should work
			So(host.SetDNSName(ctx, "hostname2"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

		})

	})
}

func TestMarkAsProvisioned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		var err error

		host := &Host{
			Id: "hostOne",
		}

		host2 := &Host{
			Id:     "hostTwo",
			Status: evergreen.HostTerminated,
		}

		host3 := &Host{
			Id:               "hostThree",
			Status:           evergreen.HostProvisioning,
			NeedsReprovision: ReprovisionToNew,
		}

		So(host.Insert(ctx), ShouldBeNil)
		So(host2.Insert(ctx), ShouldBeNil)
		So(host3.Insert(ctx), ShouldBeNil)
		Convey("marking a host that isn't down as provisioned should update the status"+
			" and provisioning fields in both the in-memory and"+
			" database copies of the host", func() {

			So(host.MarkAsProvisioned(ctx), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			So(host2.MarkAsProvisioned(ctx).Error(), ShouldContainSubstring, "not found")
			So(host2.Status, ShouldEqual, evergreen.HostTerminated)
			So(host2.Provisioned, ShouldEqual, false)
		})
	})
}

func TestMarkAsReprovisioning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"ConvertToLegacyNeedsAgent": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToLegacy
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkAsReprovisioning(ctx))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.Equal(t, utility.ZeroTime, h.AgentStartTime)
			assert.False(t, h.Provisioned)
			assert.True(t, h.NeedsNewAgent)
			assert.False(t, h.NeedsNewAgentMonitor)
		},
		"ConvertToNewNeedsAgentMonitor": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToNew
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkAsReprovisioning(ctx))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.Equal(t, utility.ZeroTime, h.AgentStartTime)
			assert.False(t, h.Provisioned)
			assert.True(t, h.NeedsNewAgentMonitor)
			assert.False(t, h.NeedsNewAgent)
		},
		"RestartJasper": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionRestartJasper
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkAsReprovisioning(ctx))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.Equal(t, utility.ZeroTime, h.AgentStartTime)
			assert.False(t, h.Provisioned)
			assert.True(t, h.NeedsNewAgentMonitor)
			assert.False(t, h.NeedsNewAgent)
		},
		"RestartJasperWithSpawnHost": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionRestartJasper
			h.StartedBy = "user"
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkAsReprovisioning(ctx))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.Equal(t, utility.ZeroTime, h.AgentStartTime)
			assert.False(t, h.Provisioned)
			assert.False(t, h.NeedsNewAgentMonitor)
		},
		"NeedsRestartJasperSetsNeedsAgentMonitor": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionRestartJasper
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkAsReprovisioning(ctx))
			assert.Equal(t, utility.ZeroTime, h.AgentStartTime)
			assert.False(t, h.Provisioned)
			assert.True(t, h.NeedsNewAgentMonitor)
			assert.False(t, h.NeedsNewAgent)
		},
		"FailsWithHostInBadStatus": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, h.MarkAsReprovisioning(ctx))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				AgentStartTime:   time.Now(),
				Provisioned:      true,
				Status:           evergreen.HostRunning,
				NeedsReprovision: ReprovisionToLegacy,
				StartedBy:        evergreen.User,
			}
			testCase(t, h)
		})
	}
}

func TestHostCreateSecret(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host with no secret", t, func() {

		require.NoError(t, db.Clear(Collection))

		host := &Host{Id: "hostOne"}
		So(host.Insert(ctx), ShouldBeNil)

		Convey("creating a secret", func() {
			So(host.Secret, ShouldEqual, "")
			So(host.CreateSecret(ctx, false), ShouldBeNil)

			Convey("should update the host in memory", func() {
				So(host.Secret, ShouldNotEqual, "")

				Convey("and in the database", func() {
					dbHost, err := FindOne(ctx, ById(host.Id))
					So(err, ShouldBeNil)
					So(dbHost.Secret, ShouldEqual, host.Secret)
				})
			})
		})
	})
}

func TestHostSetBillingStartTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	h := &Host{
		Id: "id",
	}
	require.NoError(t, h.Insert(ctx))

	now := time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	require.NoError(t, h.SetBillingStartTime(ctx, now))
	assert.True(t, now.Equal(h.BillingStartTime))

	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.True(t, now.Equal(dbHost.BillingStartTime))
}

func TestHostSetAgentStartTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	h := &Host{
		Id: "id",
	}
	require.NoError(t, h.Insert(ctx))

	now := time.Now()
	require.NoError(t, h.SetAgentStartTime(ctx))
	assert.True(t, now.Sub(h.AgentStartTime) < time.Second)

	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.True(t, now.Sub(dbHost.AgentStartTime) < time.Second)
}

func TestSetTaskGroupTeardownStartTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	h := &Host{
		Id: "id",
	}
	require.NoError(t, h.Insert(ctx))

	now := time.Now()
	assert.False(t, now.Sub(h.TaskGroupTeardownStartTime) < time.Second)
	require.NoError(t, h.SetTaskGroupTeardownStartTime(ctx))
	assert.True(t, now.Sub(h.TaskGroupTeardownStartTime) < time.Second)

	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.True(t, now.Sub(dbHost.TaskGroupTeardownStartTime) < time.Second)
}

func TestHostSetExpirationTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		initialExpirationTime := time.Now()
		memHost := &Host{
			Id:             "hostOne",
			NoExpiration:   true,
			ExpirationTime: initialExpirationTime,
		}
		So(memHost.Insert(ctx), ShouldBeNil)

		Convey("setting the expiration time for the host should change the "+
			" expiration time for both the in-memory and database"+
			" copies of the host", func() {

			dbHost, err := FindOne(ctx, ById(memHost.Id))

			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.NoExpiration, ShouldBeTrue)
			So(dbHost.NoExpiration, ShouldBeTrue)
			So(memHost.ExpirationTime.Round(time.Second).Equal(
				initialExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(dbHost.ExpirationTime.Round(time.Second).Equal(
				initialExpirationTime.Round(time.Second)), ShouldBeTrue)

			// now update the expiration time
			newExpirationTime := time.Now()
			So(memHost.SetExpirationTime(ctx, newExpirationTime), ShouldBeNil)

			dbHost, err = FindOne(ctx, ById(memHost.Id))

			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.NoExpiration, ShouldBeFalse)
			So(dbHost.NoExpiration, ShouldBeFalse)
			So(memHost.ExpirationTime.Round(time.Second).Equal(
				newExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(dbHost.ExpirationTime.Round(time.Second).Equal(
				newExpirationTime.Round(time.Second)), ShouldBeTrue)
		})
	})
}

func TestHostClearRunningAndSetLastTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		var err error
		var count int

		host := &Host{
			Id:          "hostOne",
			RunningTask: "taskId",
			StartedBy:   evergreen.User,
			Status:      evergreen.HostRunning,
		}

		So(host.Insert(ctx), ShouldBeNil)

		Convey("host statistics should properly count this host as active"+
			" but not idle", func() {
			count, err = Count(ctx, IsActive)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			count, err = Count(ctx, IsIdle)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("clearing the running task should clear the running task, pid,"+
			" and task dispatch time fields from both the in-memory and"+
			" database copies of the host", func() {

			So(host.ClearRunningAndSetLastTask(ctx, &task.Task{Id: "prevTask"}), ShouldBeNil)
			So(host.RunningTask, ShouldEqual, "")
			So(host.LastTask, ShouldEqual, "prevTask")

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)

			So(host.RunningTask, ShouldEqual, "")
			So(host.LastTask, ShouldEqual, "prevTask")

			Convey("the count of idle hosts should go up", func() {
				count, err := Count(ctx, IsIdle)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)

				Convey("but the active host count should remain the same", func() {
					count, err = Count(ctx, IsActive)
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1)
				})
			})

		})

	})
}

func TestUpdateHostRunningTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	Convey("With a host", t, func() {
		require.NoError(t, db.Clear(Collection))
		newTaskId := "newId"
		h := Host{
			Id:     "test1",
			Status: evergreen.HostRunning,
		}
		h2 := Host{
			Id:     "test2",
			Status: evergreen.HostStarting,
			Distro: distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodUserData,
				},
			},
		}
		h3 := Host{
			Id:     "test3",
			Status: evergreen.HostDecommissioned,
		}
		So(h.Insert(ctx), ShouldBeNil)
		So(h2.Insert(ctx), ShouldBeNil)
		So(h3.Insert(ctx), ShouldBeNil)
		Convey("updating the running task id should set proper fields", func() {
			So(h.UpdateRunningTaskWithContext(ctx, env, &task.Task{Id: newTaskId}), ShouldBeNil)
			found, err := FindOne(ctx, ById(h.Id))
			So(err, ShouldBeNil)
			So(found.RunningTask, ShouldEqual, newTaskId)
			runningTaskHosts, err := Find(ctx, IsRunningTask)
			So(err, ShouldBeNil)
			So(len(runningTaskHosts), ShouldEqual, 1)
		})
		Convey("updating the running task to an empty string should error out", func() {
			So(h.UpdateRunningTaskWithContext(ctx, env, &task.Task{}), ShouldNotBeNil)
		})
		Convey("updating the running task on a starting user data host should succeed", func() {
			So(h2.UpdateRunningTaskWithContext(ctx, env, &task.Task{Id: newTaskId}), ShouldBeNil)
			found, err := FindOne(ctx, ById(h2.Id))
			So(err, ShouldBeNil)
			So(found.RunningTask, ShouldEqual, newTaskId)
			runningTaskHosts, err := Find(ctx, IsRunningTask)
			So(err, ShouldBeNil)
			So(len(runningTaskHosts), ShouldEqual, 1)
		})
		Convey("updating a running task on a decommissioned host should error", func() {
			So(h3.UpdateRunningTaskWithContext(ctx, env, &task.Task{Id: newTaskId}), ShouldNotBeNil)
		})
	})
}

func TestUpsert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection))

		host := &Host{
			Id:     "hostOne",
			Host:   "host",
			User:   "user",
			Distro: distro.Distro{Id: "distro"},
			Status: evergreen.HostRunning,
		}

		var err error

		Convey("Performing a host upsert should upsert correctly", func() {
			_, err = host.Upsert(ctx)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

			host, err = FindOne(ctx, ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

		})

		Convey("Updating some fields of an already inserted host should cause "+
			"those fields to be updated ",
			func() {
				_, err := host.Upsert(ctx)
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostRunning)

				host, err = FindOne(ctx, ById(host.Id))
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostRunning)
				So(host.Host, ShouldEqual, "host")

				err = UpdateOne(
					ctx,
					bson.M{
						IdKey: host.Id,
					},
					bson.M{
						"$set": bson.M{
							StatusKey: evergreen.HostDecommissioned,
						},
					},
				)
				So(err, ShouldBeNil)

				// update the hostname and status
				host.Host = "host2"
				host.Status = evergreen.HostRunning
				_, err = host.Upsert(ctx)
				So(err, ShouldBeNil)

				// host db status should be modified
				host, err = FindOne(ctx, ById(host.Id))
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostRunning)
				So(host.Host, ShouldEqual, "host2")

			})
		Convey("Upserting a host that does not need its provisioning changed unsets the field", func() {
			So(host.Insert(ctx), ShouldBeNil)
			_, err := host.Upsert(ctx)
			So(err, ShouldBeNil)

			_, err = FindOne(ctx, bson.M{IdKey: host.Id, NeedsReprovisionKey: bson.M{"$exists": false}})
			So(err, ShouldBeNil)
		})
	})
}

func TestDecommissionHostsWithDistroId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("With a multiple hosts of different distros", t, func() {

		require.NoError(t, db.Clear(Collection))

		distroA := "distro_a"
		distroB := "distro_b"

		// Insert 10 of distro a and 10 of distro b

		for i := 0; i < 10; i++ {
			hostWithDistroA := &Host{
				Id:     fmt.Sprintf("hostA%v", i),
				Host:   "host",
				User:   "user",
				Distro: distro.Distro{Id: distroA},
				Status: evergreen.HostRunning,
			}
			hostWithDistroB := &Host{
				Id:     fmt.Sprintf("hostB%v", i),
				Host:   "host",
				User:   "user",
				Distro: distro.Distro{Id: distroB},
				Status: evergreen.HostRunning,
			}

			require.NoError(t, hostWithDistroA.Insert(ctx), "Error inserting"+
				"host into database")
			require.NoError(t, hostWithDistroB.Insert(ctx), "Error inserting"+
				"host into database")
		}

		Convey("When decommissioning hosts of type distro_a", func() {
			err := DecommissionHostsWithDistroId(ctx, distroA)
			So(err, ShouldBeNil)

			Convey("Distro should be marked as decommissioned accordingly", func() {
				hostsTypeA, err := Find(ctx, ByDistroIDs(distroA))
				So(err, ShouldBeNil)

				hostsTypeB, err := Find(ctx, ByDistroIDs(distroB))
				So(err, ShouldBeNil)
				for _, host := range hostsTypeA {

					So(host.Status, ShouldEqual, evergreen.HostDecommissioned)
				}

				for _, host := range hostsTypeB {
					So(host.Status, ShouldEqual, evergreen.HostRunning)

				}
			})

		})

	})
}

func TestFindNeedsNewAgent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("with the a given time for checking and an empty hosts collection", t, func() {
		require.NoError(t, db.Clear(Collection))
		now := time.Now()
		Convey("with a host that has no last communication time", func() {
			h := Host{
				Id:        "id",
				Status:    evergreen.HostRunning,
				StartedBy: evergreen.User,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodLegacySSH,
					},
				},
			}
			So(h.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(time.Now()))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, "id")
			Convey("after unsetting the host's lct", func() {
				err := UpdateOne(ctx, bson.M{IdKey: h.Id},
					bson.M{
						"$unset": bson.M{LastCommunicationTimeKey: 0},
					})
				So(err, ShouldBeNil)
				foundHost, err := FindOne(ctx, ById(h.Id))
				So(err, ShouldBeNil)
				So(foundHost, ShouldNotBeNil)
				hosts, err := Find(ctx, NeedsAgentDeploy(time.Now()))
				So(err, ShouldBeNil)
				So(len(hosts), ShouldEqual, 1)
				So(hosts[0].Id, ShouldEqual, h.Id)
			})
		})

		Convey("with a host with a last communication time > 10 mins", func() {
			anotherHost := Host{
				Id:                    "anotherID",
				LastCommunicationTime: now.Add(-time.Duration(20) * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodLegacySSH,
					},
				},
			}
			So(anotherHost.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, anotherHost.Id)
		})

		Convey("with a host with a normal LCT", func() {
			anotherHost := Host{
				Id:                    "testhost",
				LastCommunicationTime: now.Add(time.Duration(5) * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodLegacySSH,
					},
				},
			}
			So(anotherHost.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
			Convey("after resetting the LCT", func() {
				So(anotherHost.ResetLastCommunicated(ctx), ShouldBeNil)
				So(anotherHost.LastCommunicationTime, ShouldResemble, time.Unix(0, 0))
				h, err := Find(ctx, NeedsAgentDeploy(now))
				So(err, ShouldBeNil)
				So(len(h), ShouldEqual, 1)
				So(h[0].Id, ShouldEqual, "testhost")
			})
		})
		Convey("with a terminated host that has no LCT", func() {
			h := Host{
				Id:        "h",
				Status:    evergreen.HostTerminated,
				StartedBy: evergreen.User,
			}
			So(h.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
		Convey("with a host with that does not have a user", func() {
			h := Host{
				Id:        "h",
				Status:    evergreen.HostRunning,
				StartedBy: "anotherUser",
			}
			So(h.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
		Convey("with a legacy SSH host marked as needing a new agent", func() {
			h := Host{
				Id:            "h",
				Status:        evergreen.HostRunning,
				StartedBy:     evergreen.User,
				NeedsNewAgent: true,
				Distro:        distro.Distro{BootstrapSettings: distro.BootstrapSettings{Method: distro.BootstrapMethodLegacySSH}},
			}
			So(h.Insert(ctx), ShouldBeNil)

			hosts, err := Find(ctx, ShouldDeployAgent())
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, h.Id)
		})
		Convey("with a host with that is still provisioning, does not need reprovisioning, and has not yet communicated", func() {
			h := Host{
				Id:            "h",
				Status:        evergreen.HostProvisioning,
				StartedBy:     evergreen.User,
				NeedsNewAgent: true,
			}
			So(h.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(hosts, ShouldBeEmpty)
		})
		Convey("with a host with that is still provisioning, needs reprovisioning, and has not yet communicated", func() {
			h := Host{
				Id:               "h",
				Status:           evergreen.HostProvisioning,
				NeedsReprovision: ReprovisionToLegacy,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method: distro.BootstrapMethodLegacySSH,
					},
				},
				StartedBy:     evergreen.User,
				NeedsNewAgent: true,
			}
			So(h.Insert(ctx), ShouldBeNil)
			hosts, err := Find(ctx, NeedsAgentDeploy(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, h.Id)

			hosts, err = Find(ctx, ShouldDeployAgent())
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
	})
}

func TestSetNeedsToRestartJasper(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"SetsProvisioningFields": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.False(t, h.Provisioned)
			assert.Equal(t, ReprovisionRestartJasper, h.NeedsReprovision)
		},
		"SucceedsIfAlreadyNeedsToRestartJasper": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionRestartJasper
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.False(t, h.Provisioned)
			assert.Equal(t, ReprovisionRestartJasper, h.NeedsReprovision)
		},
		"FailsIfHostDoesNotExist": func(t *testing.T, h *Host) {
			assert.Error(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))
		},
		"FailsIfHostNotRunningOrProvisioning": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			assert.Error(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))
		},
		"FailsIfAlreadyNeedsOtherReprovisioning": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToLegacy
			require.NoError(t, h.Insert(ctx))

			assert.Error(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))
		},
		"NoopsIfLegacyProvisionedHost": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			h.Distro.BootstrapSettings.Communication = distro.CommunicationMethodLegacySSH
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, h.SetNeedsToRestartJasper(ctx, evergreen.User))

			assert.True(t, h.Provisioned)
			assert.Equal(t, ReprovisionNone, h.NeedsReprovision)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Id:          "id",
				Status:      evergreen.HostRunning,
				Provisioned: true,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
				},
			}
			testCase(t, h)
		})
	}
}

func TestSetNeedsReprovisionToNew(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"SetsProvisioningFields": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.False(t, h.Provisioned)
			assert.Equal(t, ReprovisionToNew, h.NeedsReprovision)
		},
		"SucceedsIfAlreadyNeedsReprovisionToNew": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToNew
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))
			assert.Equal(t, evergreen.HostProvisioning, h.Status)
			assert.False(t, h.Provisioned)
			assert.Equal(t, ReprovisionToNew, h.NeedsReprovision)
		},
		"FailsIfHostDoesNotExist": func(t *testing.T, h *Host) {
			assert.Error(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))
		},
		"FailsIfHostNotRunningOrProvisioning": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			assert.Error(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))
		},
		"FailsIfAlreadyNeedsReprovisioningToLegacy": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToLegacy
			require.NoError(t, h.Insert(ctx))

			assert.Error(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))
		},
		"NoopsIfLegacyProvisionedHost": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			h.Distro.BootstrapSettings.Communication = distro.CommunicationMethodLegacySSH
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetNeedsReprovisionToNew(ctx, evergreen.User))

			assert.Equal(t, evergreen.HostRunning, h.Status)
			assert.True(t, h.Provisioned)
			assert.Equal(t, ReprovisionNone, h.NeedsReprovision)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Id:          "id",
				Status:      evergreen.HostRunning,
				Provisioned: true,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
				},
			}
			testCase(t, h)
		})
	}
}

func TestNeedsAgentMonitorDeploy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"FindsNotRecentlyCommunicatedHosts": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotFindsHostsNotNeedingReprovisioning": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostProvisioning
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			assert.Len(t, hosts, 0)
		},
		"FindsHostsReprovisioning": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostProvisioning
			h.NeedsReprovision = ReprovisionToNew
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotFindRecentlyCommunicatedHosts": func(t *testing.T, h *Host) {
			h.LastCommunicationTime = time.Now()
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
		"DoesNotFindLegacyHosts": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			h.Distro.BootstrapSettings.Communication = distro.CommunicationMethodLegacySSH
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
		"DoesNotFindHostsWithoutBootstrapMethod": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = ""
			h.Distro.BootstrapSettings.Communication = ""
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, NeedsAgentMonitorDeploy(time.Now()))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodRPC,
					},
				},
				Status:    evergreen.HostRunning,
				StartedBy: evergreen.User,
			}
			testCase(t, &h)
		})
	}
}

func TestShouldDeployAgentMonitor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"NotRunningHost": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostDecommissioned
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, ShouldDeployAgentMonitor())
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"DoesNotNeedNewAgentMonitor": func(t *testing.T, h *Host) {
			h.NeedsNewAgentMonitor = false
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, ShouldDeployAgentMonitor())
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"BootstrapLegacySSH": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, ShouldDeployAgentMonitor())
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"BootstrapSSH": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, ShouldDeployAgentMonitor())
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"BootstrapUserData": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
			require.NoError(t, h.Insert(ctx))

			hosts, err := Find(ctx, ShouldDeployAgentMonitor())
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := Host{
				Id:                   "h",
				Status:               evergreen.HostRunning,
				StartedBy:            evergreen.User,
				NeedsNewAgentMonitor: true,
			}
			testCase(t, &h)
		})
	}
}

func TestFindUserDataSpawnHostsProvisioning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"ReturnsHostsStartedButNotRunning": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"IgnoresNonUserDataBootstrap": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresUnprovisionedHosts": func(t *testing.T, h *Host) {
			h.Provisioned = false
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresHostsSpawnedByEvergreen": func(t *testing.T, h *Host) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresRunningSpawnHosts": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostRunning
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresSpawnHostsThatProvisionForTooLong": func(t *testing.T, h *Host) {
			h.ProvisionTime = time.Now().Add(-24 * time.Hour)
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindUserDataSpawnHostsProvisioning(ctx)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := Host{
				Id:            "host_id",
				Status:        evergreen.HostStarting,
				Provisioned:   true,
				ProvisionTime: time.Now(),
				StartedBy:     "user",
				Distro: distro.Distro{
					Id: "distro_id",
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodSSH,
					},
				},
			}
			testCase(t, &h)
		})
	}
}

func TestGenerateAndSaveJasperCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checkCredentialsAreSet := func(t *testing.T, h *Host, creds *certdepot.Credentials) {
		assert.NotZero(t, creds.CACert)
		assert.NotZero(t, creds.Cert)
		assert.NotZero(t, creds.Key)
		assert.Equal(t, h.Id, h.JasperCredentialsID)
		assert.Equal(t, h.JasperCredentialsID, creds.ServerName)
	}
	generateAndSaveCreds := func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) *certdepot.Credentials {
		require.NoError(t, h.Insert(ctx))
		creds, err := h.GenerateJasperCredentials(ctx, env)
		require.NoError(t, err)
		assert.NotZero(t, h.JasperCredentialsID)
		checkCredentialsAreSet(t, h, creds)
		require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))
		return creds
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host){
		"FailsWithoutHostInDB": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			creds, err := h.GenerateJasperCredentials(ctx, env)
			assert.Error(t, err)
			assert.Zero(t, creds)
		},
		"Succeeds": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			generateAndSaveCreds(ctx, t, env, h)
			cert, err := certdepot.GetCertificate(env.CertificateDepot(), h.JasperCredentialsID)
			require.NoError(t, err)
			rawCert, err := cert.GetRawCertificate()
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, rawCert.Subject.CommonName)
			assert.Equal(t, []string{h.JasperCredentialsID}, rawCert.DNSNames)
		},
		"SucceedsWithHostnameDifferentFromID": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			h.Host = "127.0.0.1"
			generateAndSaveCreds(ctx, t, env, h)
			cert, err := certdepot.GetCertificate(env.CertificateDepot(), h.JasperCredentialsID)
			require.NoError(t, err)
			rawCert, err := cert.GetRawCertificate()
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, rawCert.Subject.CommonName)
			assert.Equal(t, []string{h.JasperCredentialsID}, rawCert.DNSNames)
			require.Len(t, rawCert.IPAddresses, 1)
			assert.Equal(t, h.Host, rawCert.IPAddresses[0].String())
		},
		"SucceedsWithIDAsIPAddress": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			h.Id = "127.0.0.1"
			generateAndSaveCreds(ctx, t, env, h)
			cert, err := certdepot.GetCertificate(env.CertificateDepot(), h.JasperCredentialsID)
			require.NoError(t, err)
			rawCert, err := cert.GetRawCertificate()
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, rawCert.Subject.CommonName)
			assert.Equal(t, []string{h.JasperCredentialsID}, rawCert.DNSNames)
			require.Len(t, rawCert.IPAddresses, 1)
			assert.Equal(t, h.JasperCredentialsID, rawCert.IPAddresses[0].String())
		},
		"SucceedsWithIPAddress": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			h.IP = "2600:1f18:ae9:dd00:d071:8752:6e2b:fc6a"
			generateAndSaveCreds(ctx, t, env, h)
			cert, err := certdepot.GetCertificate(env.CertificateDepot(), h.JasperCredentialsID)
			require.NoError(t, err)
			rawCert, err := cert.GetRawCertificate()
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, rawCert.Subject.CommonName)
			assert.Equal(t, []string{h.JasperCredentialsID}, rawCert.DNSNames)
			require.Len(t, rawCert.IPAddresses, 1)
			assert.Equal(t, h.IP, rawCert.IPAddresses[0].String())
		},
		"SucceedsWithDuplicateIDAndHostname": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			h.Id = "127.0.0.1"
			h.Host = "127.0.0.1"
			generateAndSaveCreds(ctx, t, env, h)
			cert, err := certdepot.GetCertificate(env.CertificateDepot(), h.JasperCredentialsID)
			require.NoError(t, err)
			rawCert, err := cert.GetRawCertificate()
			require.NoError(t, err)
			assert.Equal(t, h.JasperCredentialsID, rawCert.Subject.CommonName)
			assert.Equal(t, []string{h.JasperCredentialsID}, rawCert.DNSNames)
			require.Len(t, rawCert.IPAddresses, 1)
			assert.Equal(t, h.JasperCredentialsID, rawCert.IPAddresses[0].String())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
			defer tcancel()
			require.NoError(t, db.ClearCollections(evergreen.CredentialsCollection, Collection))
			defer func() {
				assert.NoError(t, db.ClearCollections(evergreen.CredentialsCollection, Collection))
			}()
			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			testCase(tctx, t, env, &Host{Id: "id"})
		})
	}
}

func TestFindByShouldConvertProvisioning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"IgnoresHostWithoutMatchingStatus": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostTerminated
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByShouldConvertProvisioning(ctx)
			assert.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"ReturnsHostsThatNeedNewReprovisioning": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByShouldConvertProvisioning(ctx)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"ReturnsHostsThatNeedLegacyReprovisioning": func(t *testing.T, h *Host) {
			h.NeedsReprovision = ReprovisionToLegacy
			h.NeedsNewAgentMonitor = false
			h.NeedsNewAgent = true
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindByShouldConvertProvisioning(ctx)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Id:                   "id",
				StartedBy:            evergreen.User,
				Status:               evergreen.HostProvisioning,
				NeedsReprovision:     ReprovisionToNew,
				NeedsNewAgentMonitor: true,
			}
			testCase(t, h)
		})
	}
}

func TestHostElapsedCommTime(t *testing.T) {
	assert := assert.New(t)
	now := time.Now()
	hostThatRanTask := Host{
		Id:                    "hostThatRanTask",
		CreationTime:          now.Add(-30 * time.Minute),
		StartTime:             now.Add(-20 * time.Minute),
		LastCommunicationTime: now.Add(-10 * time.Minute),
	}
	hostThatJustStarted := Host{
		Id:           "hostThatJustStarted",
		CreationTime: now.Add(-5 * time.Minute),
		StartTime:    now.Add(-1 * time.Minute),
	}
	hostWithNoCreateTime := Host{
		Id:                    "hostWithNoCreateTime",
		LastCommunicationTime: now.Add(-15 * time.Minute),
	}
	hostWithOnlyCreateTime := Host{
		Id:           "hostWithOnlyCreateTime",
		CreationTime: now.Add(-7 * time.Minute),
	}
	hostThatsRunningTeardown := Host{
		Id:                         "hostThatsRunningTeardown",
		CreationTime:               now.Add(-7 * time.Minute),
		TaskGroupTeardownStartTime: now.Add(-1 * time.Minute),
	}

	assert.InDelta(int64(10*time.Minute), int64(hostThatRanTask.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(1*time.Minute), int64(hostThatJustStarted.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(15*time.Minute), int64(hostWithNoCreateTime.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(7*time.Minute), int64(hostWithOnlyCreateTime.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.Zero(hostThatsRunningTeardown.GetElapsedCommunicationTime())
}

func TestHostUpsert(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	const hostID = "upsertTest"
	testHost := &Host{
		Id:             hostID,
		Host:           "dns",
		User:           "user",
		UserHost:       true,
		Distro:         distro.Distro{Id: "distro1"},
		Provisioned:    true,
		StartedBy:      "started_by",
		ExpirationTime: time.Now().Round(time.Second),
		Provider:       "provider",
		Tag:            "tag",
		InstanceType:   "instance",
		Zone:           "zone",
		ProvisionOptions: &ProvisionOptions{
			TaskId:   "task_id",
			TaskSync: true,
		},
		ContainerImages: map[string]bool{},
	}

	// test inserting new host
	_, err := testHost.Upsert(ctx)
	assert.NoError(err)
	hostFromDB, err := FindOne(ctx, ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(testHost, hostFromDB)

	// test updating the same host
	testHost.User = "user2"
	_, err = testHost.Upsert(ctx)
	assert.NoError(err)
	hostFromDB, err = FindOne(ctx, ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(testHost.User, hostFromDB.User)

	// test updating a field that is not upserted
	testHost.Secret = "secret"
	_, err = testHost.Upsert(ctx)
	assert.NoError(err)
	hostFromDB, err = FindOne(ctx, ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.NotEqual(testHost.Secret, hostFromDB.Secret)

	assert.Equal(testHost.ProvisionOptions, hostFromDB.ProvisionOptions)
}

func TestHostStats(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)

	const d1 = "distro1"
	const d2 = "distro2"

	require.NoError(t, db.Clear(Collection))
	host1 := &Host{
		Id:          "host1",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task",
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostStarting,
	}
	host3 := &Host{
		Id:     "host3",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostTerminated,
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task2",
	}
	host5 := &Host{
		Id:     "host5",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:          "host7",
		Distro:      distro.Distro{Id: d2},
		Status:      evergreen.HostRunning,
		RunningTask: "task3",
	}
	host8 := &Host{
		Id:     "host8",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostRunning,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	// test GetStatsByDistro
	stats, err := GetStatsByDistro()
	assert.NoError(err)
	for _, entry := range stats {
		if entry.Distro == d1 {
			if entry.Status == evergreen.HostRunning {
				assert.Equal(2, entry.Count)
				assert.Equal(2, entry.NumTasks)
			} else if entry.Status == evergreen.HostStarting {
				assert.Equal(1, entry.Count)
				assert.Equal(0, entry.NumTasks)
			}
		} else if entry.Distro == d2 {
			if entry.Status == evergreen.HostRunning {
				assert.Equal(2, entry.Count)
				assert.Equal(1, entry.NumTasks)
			} else if entry.Status == evergreen.HostProvisioning {
				assert.Equal(2, entry.Count)
				assert.Equal(0, entry.NumTasks)
			}
		}
	}
}

func TestHostFindingWithTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, task.Collection))
	assert := assert.New(t)
	task1 := task.Task{
		Id: "task1",
	}
	task2 := task.Task{
		Id: "task2",
	}
	task3 := task.Task{
		Id: "task3",
	}
	host1 := Host{
		Id:          "host1",
		RunningTask: task1.Id,
		Status:      evergreen.HostRunning,
	}
	host2 := Host{
		Id:          "host2",
		RunningTask: task2.Id,
		Status:      evergreen.HostRunning,
	}
	host3 := Host{
		Id:          "host3",
		RunningTask: "",
		Status:      evergreen.HostRunning,
	}
	host4 := Host{
		Id:     "host4",
		Status: evergreen.HostTerminated,
	}
	assert.NoError(task1.Insert())
	assert.NoError(task2.Insert())
	assert.NoError(task3.Insert())
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))

	hosts, err := FindRunningHosts(ctx, true)
	assert.NoError(err)

	assert.Equal(3, len(hosts))
	assert.Equal(task1.Id, hosts[0].RunningTaskFull.Id)
	assert.Equal(task2.Id, hosts[1].RunningTaskFull.Id)
	assert.Nil(hosts[2].RunningTaskFull)
}

func TestInactiveHostCountPipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	assert := assert.New(t)

	h1 := Host{
		Id:       "h1",
		Status:   evergreen.HostRunning,
		Provider: evergreen.HostTypeStatic,
	}
	assert.NoError(h1.Insert(ctx))
	h2 := Host{
		Id:       "h2",
		Status:   evergreen.HostQuarantined,
		Provider: evergreen.HostTypeStatic,
	}
	assert.NoError(h2.Insert(ctx))
	h3 := Host{
		Id:       "h3",
		Status:   evergreen.HostDecommissioned,
		Provider: "notstatic",
	}
	assert.NoError(h3.Insert(ctx))
	h4 := Host{
		Id:       "h4",
		Status:   evergreen.HostRunning,
		Provider: "notstatic",
	}
	assert.NoError(h4.Insert(ctx))
	h5 := Host{
		Id:       "h5",
		Status:   evergreen.HostQuarantined,
		Provider: "notstatic",
	}
	assert.NoError(h5.Insert(ctx))

	var out []InactiveHostCounts
	err := db.Aggregate(Collection, inactiveHostCountPipeline(), &out)
	assert.NoError(err)
	assert.Len(out, 2)
	for _, count := range out {
		if count.HostType == evergreen.HostTypeStatic {
			assert.Equal(1, count.Count)
		} else {
			assert.Equal(2, count.Count)
		}
	}
}

func setupIdleHostQueryIndex(t *testing.T) {
	require.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{
		Keys: StartedByStatusIndex,
	}))
}

func TestIdleEphemeralGroupedByDistroID(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.ClearCollections(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := mock.Environment{}
	assert.NoError(env.Configure(ctx))

	setupIdleHostQueryIndex(t)

	defer func() {
		assert.NoError(db.DropCollections(Collection))
	}()

	const (
		d1 = "distro1"
		d2 = "distro2"
		d3 = "distro3"
	)

	host1 := &Host{
		Id:            "host1",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-20 * time.Minute),
	}
	host2 := &Host{
		Id:            "host2",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-10 * time.Minute),
	}
	host3 := &Host{
		Id:            "host3",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-30 * time.Minute),
	}
	host4 := &Host{
		Id:            "host4",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-40 * time.Minute),
	}
	host5 := &Host{
		Id:            "host5",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-50 * time.Minute),
	}
	host6 := &Host{
		Id:            "host6",
		Distro:        distro.Distro{Id: d1},
		RunningTask:   "I'm running a task so I'm certainly not idle!",
		Status:        evergreen.HostRunning,
		StartedBy:     evergreen.User,
		Provider:      evergreen.ProviderNameMock,
		HasContainers: false,
		CreationTime:  time.Now().Add(-60 * time.Minute),
	}
	// User data host that is running task and recently communicated is not
	// idle.
	host7 := &Host{
		Id: "host7",
		Distro: distro.Distro{
			Id: d3,
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodUserData,
			},
		},
		LastCommunicationTime: time.Now(),
		RunningTask:           "running_task",
		StartedBy:             evergreen.User,
		Provider:              evergreen.ProviderNameMock,
		Status:                evergreen.HostStarting,
		CreationTime:          time.Now().Add(-60 * time.Minute),
		AgentStartTime:        time.Now(),
	}
	// User data host that is not running a task is idle if its AgentStartTime is set.
	host8 := &Host{
		Id: "host8",
		Distro: distro.Distro{
			Id: d3,
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodUserData,
			},
		},
		LastCommunicationTime: time.Now().Add(-time.Hour),
		StartedBy:             evergreen.User,
		Provider:              evergreen.ProviderNameMock,
		Status:                evergreen.HostStarting,
		CreationTime:          time.Now(),
		AgentStartTime:        time.Now(),
	}
	// User data host that is running task but has not communicated recently is
	// not idle.
	host9 := &Host{
		Id: "host9",
		Distro: distro.Distro{
			Id: d3,
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodUserData,
			},
		},
		RunningTask:           "running_task_for_long_time",
		LastCommunicationTime: time.Now().Add(-time.Hour),
		StartedBy:             evergreen.User,
		Provider:              evergreen.ProviderNameMock,
		Status:                evergreen.HostStarting,
		CreationTime:          time.Now().Add(-100 * time.Minute),
		AgentStartTime:        time.Now(),
	}
	// User data host that is not running task but has not had its
	// AgentStartTime set is not idle.
	host10 := &Host{
		Id: "host10",
		Distro: distro.Distro{
			Id: d3,
			BootstrapSettings: distro.BootstrapSettings{
				Method: distro.BootstrapMethodUserData,
			},
		},
		LastCommunicationTime: time.Now(),
		StartedBy:             evergreen.User,
		Provider:              evergreen.ProviderNameMock,
		Status:                evergreen.HostStarting,
		CreationTime:          time.Now(),
	}

	require.NoError(host1.Insert(ctx))
	require.NoError(host2.Insert(ctx))
	require.NoError(host3.Insert(ctx))
	require.NoError(host4.Insert(ctx))
	require.NoError(host5.Insert(ctx))
	require.NoError(host6.Insert(ctx))
	require.NoError(host7.Insert(ctx))
	require.NoError(host8.Insert(ctx))
	require.NoError(host9.Insert(ctx))
	require.NoError(host10.Insert(ctx))

	idleHostsByDistroID, err := IdleEphemeralGroupedByDistroID(ctx, &env)
	assert.NoError(err)
	assert.Len(idleHostsByDistroID, 3)

	// Confirm the hosts are sorted from oldest to newest CreationTime.
	for _, distroHosts := range idleHostsByDistroID {
		switch distroHosts.DistroID {
		case d1:
			require.Len(distroHosts.IdleHosts, 3)
			assert.Equal("host4", distroHosts.IdleHosts[0].Id)
			assert.Equal("host1", distroHosts.IdleHosts[1].Id)
			assert.Equal("host2", distroHosts.IdleHosts[2].Id)
		case d2:
			require.Len(distroHosts.IdleHosts, 2)
			assert.Equal("host5", distroHosts.IdleHosts[0].Id)
			assert.Equal("host3", distroHosts.IdleHosts[1].Id)
		case d3:
			require.Len(distroHosts.IdleHosts, 1)
			assert.Equal("host8", distroHosts.IdleHosts[0].Id)
		}
	}
}

func TestFindAllRunningContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:          "host1",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task",
		ParentID:    "parentId",
	}
	host2 := &Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostStarting,
		ParentID: "parentId",
	}
	host3 := &Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostTerminated,
		ParentID: "parentId",
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task2",
		ParentID:    "parentId",
	}
	host5 := &Host{
		Id:       "host5",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "parentId",
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:          "host7",
		Distro:      distro.Distro{Id: d2},
		Status:      evergreen.HostRunning,
		RunningTask: "task3",
	}
	host8 := &Host{
		Id:     "host8",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostRunning,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	containers, err := FindAllRunningContainers(ctx)
	assert.NoError(err)
	assert.Equal(2, len(containers))
}

func TestFindAllRunningContainersEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:          "host1",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task",
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostStarting,
	}
	host3 := &Host{
		Id:     "host3",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostTerminated,
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task2",
	}
	host5 := &Host{
		Id:     "host5",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:          "host7",
		Distro:      distro.Distro{Id: d2},
		Status:      evergreen.HostRunning,
		RunningTask: "task3",
	}
	host8 := &Host{
		Id:     "host8",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostRunning,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	containers, err := FindAllRunningContainers(ctx)
	assert.NoError(err)
	assert.Empty(containers)
}

func TestFindAllRunningParents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:            "host1",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostStarting,
	}
	host3 := &Host{
		Id:     "host3",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostTerminated,
	}
	host4 := &Host{
		Id:            "host4",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host5 := &Host{
		Id:       "host5",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "parentId",
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:            "host7",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host8 := &Host{
		Id:            "host8",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostTerminated,
		HasContainers: true,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	hosts, err := FindAllRunningParents(ctx)
	assert.NoError(err)
	assert.Equal(3, len(hosts))

}

func TestFindAllRunningParentsOrdered(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:                      "host1",
		Distro:                  distro.Distro{Id: d1},
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(10 * time.Minute),
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostStarting,
	}
	host3 := &Host{
		Id:     "host3",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostTerminated,
	}
	host4 := &Host{
		Id:                      "host4",
		Distro:                  distro.Distro{Id: d1},
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(30 * time.Minute),
	}
	host5 := &Host{
		Id:                      "host5",
		Distro:                  distro.Distro{Id: d2},
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(5 * time.Minute),
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:                      "host7",
		Distro:                  distro.Distro{Id: d2},
		Status:                  evergreen.HostRunning,
		HasContainers:           true,
		LastContainerFinishTime: time.Now().Add(15 * time.Minute),
	}

	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))

	hosts, err := FindAllRunningParentsOrdered(ctx)
	assert.NoError(err)
	assert.Equal(hosts[0].Id, host5.Id)
	assert.Equal(hosts[1].Id, host1.Id)
	assert.Equal(hosts[2].Id, host7.Id)
	assert.Equal(hosts[3].Id, host4.Id)

}

func TestFindAllRunningParentsEmpty(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:     "host1",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostRunning,
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostStarting,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))

	containers, err := FindAllRunningParents(ctx)
	assert.NoError(err)
	assert.Empty(containers)
}

func TestGetContainers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:            "host1",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		RunningTask:   "task",
		HasContainers: true,
	}
	host2 := &Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostStarting,
		ParentID: "parentId1",
	}
	host3 := &Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task2",
		ParentID:    "parentId1",
	}
	host5 := &Host{
		Id:       "host5",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "host1",
	}
	host6 := &Host{
		Id:       "host6",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "host1",
	}
	host7 := &Host{
		Id:          "host7",
		Distro:      distro.Distro{Id: d2},
		Status:      evergreen.HostRunning,
		RunningTask: "task3",
		ParentID:    "host1",
	}
	host8 := &Host{
		Id:       "host8",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	containers, err := host1.GetContainers(ctx)
	assert.NoError(err)
	assert.Equal(5, len(containers))
}

func TestGetContainersNotParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:          "host1",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task",
	}
	host2 := &Host{
		Id:       "host2",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostStarting,
		ParentID: "parentId1",
	}
	host3 := &Host{
		Id:       "host3",
		Distro:   distro.Distro{Id: d1},
		Status:   evergreen.HostTerminated,
		ParentID: "parentId",
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: d1},
		Status:      evergreen.HostRunning,
		RunningTask: "task2",
		ParentID:    "parentId1",
	}
	host5 := &Host{
		Id:       "host5",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "parentId",
	}
	host6 := &Host{
		Id:       "host6",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "parentId",
	}
	host7 := &Host{
		Id:          "host7",
		Distro:      distro.Distro{Id: d2},
		Status:      evergreen.HostRunning,
		RunningTask: "task3",
		ParentID:    "parentId",
	}
	host8 := &Host{
		Id:       "host8",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostRunning,
		ParentID: "parentId",
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	containers, err := host1.GetContainers(ctx)
	assert.Error(err)
	assert.Empty(containers)
}

func TestIsIdleParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	provisionTimeRecent := time.Now().Add(-5 * time.Minute)
	provisionTimeOld := time.Now().Add(-1 * time.Hour)

	host1 := &Host{
		Id:            "host1",
		Status:        evergreen.HostRunning,
		ProvisionTime: provisionTimeOld,
	}
	host2 := &Host{
		Id:            "host2",
		Status:        evergreen.HostRunning,
		HasContainers: true,
		ProvisionTime: provisionTimeRecent,
	}
	host3 := &Host{
		Id:            "host3",
		Status:        evergreen.HostRunning,
		HasContainers: true,
		ProvisionTime: provisionTimeOld,
	}
	host4 := &Host{
		Id:            "host4",
		Status:        evergreen.HostRunning,
		HasContainers: true,
		ProvisionTime: provisionTimeOld,
	}
	host5 := &Host{
		Id:       "host5",
		Status:   evergreen.HostTerminated,
		ParentID: "host3",
	}
	host6 := &Host{
		Id:       "host6",
		Status:   evergreen.HostDecommissioned,
		ParentID: "host4",
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))

	// does not have containers --> false
	idle, err := host1.IsIdleParent(ctx)
	assert.False(idle)
	assert.NoError(err)

	// recent provision time --> false
	idle, err = host2.IsIdleParent(ctx)
	assert.False(idle)
	assert.NoError(err)

	// old provision time --> true
	idle, err = host3.IsIdleParent(ctx)
	assert.True(idle)
	assert.NoError(err)

	// has decommissioned container --> true
	idle, err = host4.IsIdleParent(ctx)
	assert.True(idle)
	assert.NoError(err)

	// is a container --> false
	idle, err = host5.IsIdleParent(ctx)
	assert.False(idle)
	assert.NoError(err)

}

func TestUpdateParentIDs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.Clear(Collection))
	parent := Host{
		Id:            "parent",
		Tag:           "foo",
		HasContainers: true,
	}
	assert.NoError(parent.Insert(ctx))
	container1 := Host{
		Id:       "c1",
		ParentID: parent.Tag,
	}
	assert.NoError(container1.Insert(ctx))
	container2 := Host{
		Id:       "c2",
		ParentID: parent.Tag,
	}
	assert.NoError(container2.Insert(ctx))

	assert.NoError(parent.UpdateParentIDs(ctx))
	dbContainer1, err := FindOneId(ctx, container1.Id)
	assert.NoError(err)
	assert.Equal(parent.Id, dbContainer1.ParentID)
	dbContainer2, err := FindOneId(ctx, container2.Id)
	assert.NoError(err)
	assert.Equal(parent.Id, dbContainer2.ParentID)
}

func TestFindParentOfContainer(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host1 := &Host{
		Id:       "host1",
		Host:     "host",
		User:     "user",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "parentId",
	}
	host2 := &Host{
		Id:            "parentId",
		Distro:        distro.Distro{Id: "distro"},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}

	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))

	parent, err := host1.GetParent(ctx)
	assert.NoError(err)
	assert.NotNil(parent)
}

func TestFindParentOfContainerNoParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host := &Host{
		Id:     "hostOne",
		Host:   "host",
		User:   "user",
		Distro: distro.Distro{Id: "distro"},
		Status: evergreen.HostRunning,
	}

	assert.NoError(host.Insert(ctx))

	parent, err := host.GetParent(ctx)
	assert.Error(err)
	assert.Nil(parent)
}

func TestFindParentOfContainerCannotFindParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host := &Host{
		Id:       "hostOne",
		Host:     "host",
		User:     "user",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "parentId",
	}

	assert.NoError(host.Insert(ctx))

	parent, err := host.GetParent(ctx)
	require.Error(t, err)
	assert.Contains(err.Error(), "not found")
	assert.Nil(parent)
}

func TestFindParentOfContainerNotParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host1 := &Host{
		Id:       "hostOne",
		Host:     "host",
		User:     "user",
		Distro:   distro.Distro{Id: "distro"},
		Status:   evergreen.HostRunning,
		ParentID: "parentId",
	}

	host2 := &Host{
		Id:     "parentId",
		Distro: distro.Distro{Id: "distro"},
		Status: evergreen.HostRunning,
	}

	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))

	parent, err := host1.GetParent(ctx)
	assert.Error(err)
	assert.Nil(parent)
}

func TestLastContainerFinishTimePipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, task.Collection))
	assert := assert.New(t)

	startTimeOne := time.Now()
	startTimeTwo := time.Now().Add(-10 * time.Minute)
	startTimeThree := time.Now().Add(-1 * time.Hour)

	durationOne := 5 * time.Minute
	durationTwo := 30 * time.Minute
	durationThree := 2 * time.Hour

	h1 := Host{
		Id:          "h1",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t1",
	}
	assert.NoError(h1.Insert(ctx))
	h2 := Host{
		Id:          "h2",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t2",
	}
	assert.NoError(h2.Insert(ctx))
	h3 := Host{
		Id:          "h3",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t3",
	}
	assert.NoError(h3.Insert(ctx))
	h4 := Host{
		Id:          "h4",
		Status:      evergreen.HostRunning,
		RunningTask: "t4",
	}
	assert.NoError(h4.Insert(ctx))
	h5 := Host{
		Id:          "h5",
		Status:      evergreen.HostRunning,
		ParentID:    "p2",
		RunningTask: "t5",
	}
	assert.NoError(h5.Insert(ctx))
	h6 := Host{
		Id:          "h6",
		Status:      evergreen.HostRunning,
		ParentID:    "p2",
		RunningTask: "t6",
	}
	assert.NoError(h6.Insert(ctx))
	t1 := task.Task{
		Id: "t1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		},
		StartTime: startTimeOne,
	}
	assert.NoError(t1.Insert())
	t2 := task.Task{
		Id: "t2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		},
		StartTime: startTimeTwo,
	}
	assert.NoError(t2.Insert())
	t3 := task.Task{
		Id: "t3",
		DurationPrediction: util.CachedDurationValue{
			Value: durationThree,
		},
		StartTime: startTimeThree,
	}
	assert.NoError(t3.Insert())
	t4 := task.Task{
		Id: "t4",
		DurationPrediction: util.CachedDurationValue{
			Value: durationThree,
		},
		StartTime: startTimeOne,
	}
	assert.NoError(t4.Insert())
	t5 := task.Task{
		Id: "t5",
		DurationPrediction: util.CachedDurationValue{
			Value: durationThree,
		},
		StartTime: startTimeOne,
	}
	assert.NoError(t5.Insert())
	t6 := task.Task{
		Id: "t6",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		},
		StartTime: startTimeOne,
	}
	assert.NoError(t6.Insert())

	var out []FinishTime
	var results = make(map[string]time.Time)

	err := db.Aggregate(Collection, lastContainerFinishTimePipeline(), &out)
	assert.NoError(err)

	for _, doc := range out {
		results[doc.Id] = doc.FinishTime
	}

	// checks if last container finish time for each parent is within millisecond of expected
	// necessary because Go uses nanoseconds while MongoDB uses milliseconds
	assert.WithinDuration(results["p1"], startTimeThree.Add(durationThree), time.Millisecond)
	assert.WithinDuration(results["p2"], startTimeOne.Add(durationThree), time.Millisecond)
}

func TestFindHostsSpawnedByTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(Collection))

	hosts := []*Host{
		{
			Id:     "1",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TaskID:        "task_1",
				BuildID:       "build_1",
				SpawnedByTask: true,
			},
		},
		{
			Id:     "2",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "3",
			Status: evergreen.HostRunning,
		},
		{
			Id:     "4",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TaskID:        "task_2",
				BuildID:       "build_1",
				SpawnedByTask: true,
			},
		},
		{
			Id:     "5",
			Status: evergreen.HostDecommissioned,
			SpawnOptions: SpawnOptions{
				TaskID:        "task_1",
				BuildID:       "build_1",
				SpawnedByTask: true,
			},
		},
		{
			Id:     "6",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TaskID:        "task_2",
				BuildID:       "build_1",
				SpawnedByTask: true,
			},
		},
		{
			Id:     "7",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TaskID:              "task_1",
				BuildID:             "build_1",
				SpawnedByTask:       true,
				TaskExecutionNumber: 1,
			},
		},
	}
	for i := range hosts {
		require.NoError(hosts[i].Insert(ctx))
	}
	found, err := FindAllHostsSpawnedByTasks(ctx)
	assert.NoError(err)
	assert.Len(found, 3)
	assert.Equal(found[0].Id, "1")
	assert.Equal(found[1].Id, "4")
	assert.Equal(found[2].Id, "7")

	found, err = FindHostsSpawnedByTask(ctx, "task_1", 0, []string{evergreen.HostRunning})
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal(found[0].Id, "1")

	found, err = FindHostsSpawnedByTask(ctx, "task_1", 1, []string{evergreen.HostRunning})
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal(found[0].Id, "7")

	found, err = FindHostsSpawnedByBuild(ctx, "build_1")
	assert.NoError(err)
	assert.Len(found, 3)
	assert.Equal(found[0].Id, "1")
	assert.Equal(found[1].Id, "4")
	assert.Equal(found[2].Id, "7")
}

func TestCountContainersOnParents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	h1 := Host{
		Id:            "h1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h2 := Host{
		Id:            "h2",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h3 := Host{
		Id:            "h3",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h4 := Host{
		Id:       "h4",
		Status:   evergreen.HostRunning,
		ParentID: "h1",
	}
	h5 := Host{
		Id:       "h5",
		Status:   evergreen.HostRunning,
		ParentID: "h1",
	}
	h6 := Host{
		Id:       "h6",
		Status:   evergreen.HostRunning,
		ParentID: "h2",
	}
	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))
	assert.NoError(h4.Insert(ctx))
	assert.NoError(h5.Insert(ctx))
	assert.NoError(h6.Insert(ctx))

	c1, err := HostGroup{h1, h2}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c1, 3)

	c2, err := HostGroup{h1, h3}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c2, 2)

	c3, err := HostGroup{h2, h3}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c3, 1)

	// Parents have no containers
	c4, err := HostGroup{h3}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c4, 0)

	// Parents are actually containers
	c5, err := HostGroup{h4, h5, h6}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c5, 0)

	// Parents list is empty
	c6, err := HostGroup{}.CountContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal(c6, 0)
}

func TestFindUphostContainersOnParents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	h1 := Host{
		Id:            "h1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h2 := Host{
		Id:            "h2",
		Status:        evergreen.HostTerminated,
		HasContainers: true,
	}
	h3 := Host{
		Id:            "h3",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h4 := Host{
		Id:       "h4",
		Status:   evergreen.HostRunning,
		ParentID: "h1",
	}
	h5 := Host{
		Id:       "h5",
		Status:   evergreen.HostRunning,
		ParentID: "h1",
	}
	h6 := Host{
		Id:       "h6",
		Status:   evergreen.HostTerminated,
		ParentID: "h2",
	}
	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))
	assert.NoError(h4.Insert(ctx))
	assert.NoError(h5.Insert(ctx))
	assert.NoError(h6.Insert(ctx))

	hosts1, err := HostGroup{h1, h2, h3}.FindUphostContainersOnParents(ctx)
	assert.NoError(err)
	assert.Equal([]Host{h4, h5}, hosts1)

	// Parents have no containers
	hosts2, err := HostGroup{h3}.FindUphostContainersOnParents(ctx)
	assert.NoError(err)
	assert.Empty(hosts2)

	// Parents are actually containers
	hosts3, err := HostGroup{h4, h5, h6}.FindUphostContainersOnParents(ctx)
	assert.NoError(err)
	assert.Empty(hosts3)

}

func TestGetHostIds(t *testing.T) {
	assert := assert.New(t)
	hosts := HostGroup{
		Host{
			Id: "h1",
		},
		Host{
			Id: "h2",
		},
		Host{
			Id: "h3",
		},
	}
	ids := hosts.GetHostIds()
	assert.Equal([]string{"h1", "h2", "h3"}, ids)
}

func TestFindAllRunningParentsByDistroID(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

	host1 := &Host{
		Id:            "host1",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host2 := &Host{
		Id:     "host2",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostStarting,
	}
	host3 := &Host{
		Id:     "host3",
		Distro: distro.Distro{Id: d1},
		Status: evergreen.HostTerminated,
	}
	host4 := &Host{
		Id:            "host4",
		Distro:        distro.Distro{Id: d1},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host5 := &Host{
		Id:       "host5",
		Distro:   distro.Distro{Id: d2},
		Status:   evergreen.HostProvisioning,
		ParentID: "parentId",
	}
	host6 := &Host{
		Id:     "host6",
		Distro: distro.Distro{Id: d2},
		Status: evergreen.HostProvisioning,
	}
	host7 := &Host{
		Id:            "host7",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	host8 := &Host{
		Id:            "host8",
		Distro:        distro.Distro{Id: d2},
		Status:        evergreen.HostTerminated,
		HasContainers: true,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(host5.Insert(ctx))
	assert.NoError(host6.Insert(ctx))
	assert.NoError(host7.Insert(ctx))
	assert.NoError(host8.Insert(ctx))

	parents, err := FindAllRunningParentsByDistroID(ctx, d1)
	assert.NoError(err)
	assert.Equal(2, len(parents))
}

func TestFindUphostParentsByContainerPool(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host1 := &Host{
		Id:            "host1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
		ContainerPoolSettings: &evergreen.ContainerPool{
			Distro:        "d1",
			Id:            "test-pool",
			MaxContainers: 100,
		},
	}
	host2 := &Host{
		Id:            "host2",
		Status:        evergreen.HostTerminated,
		HasContainers: true,
		ContainerPoolSettings: &evergreen.ContainerPool{
			Distro:        "d1",
			Id:            "test-pool",
			MaxContainers: 10,
		},
	}
	host3 := &Host{
		Id:     "host3",
		Status: evergreen.HostRunning,
	}
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))

	hosts, err := findUphostParentsByContainerPool(ctx, "test-pool")
	assert.NoError(err)
	assert.Equal([]Host{*host1}, hosts)

	hosts, err = findUphostParentsByContainerPool(ctx, "missing-test-pool")
	assert.NoError(err)
	assert.Empty(hosts)
}

func TestHostsSpawnedByTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := require.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(Collection, task.Collection, build.Collection))
	finishedTask := &task.Task{
		Id:     "running_task",
		Status: evergreen.TaskSucceeded,
	}
	require.NoError(finishedTask.Insert())
	finishedTaskNewExecution := &task.Task{
		Id:        "restarted_task",
		Status:    evergreen.TaskStarted,
		Execution: 1,
	}
	require.NoError(finishedTaskNewExecution.Insert())
	finishedBuild := &build.Build{
		Id:     "running_build",
		Status: evergreen.BuildSucceeded,
	}
	require.NoError(finishedBuild.Insert())
	hosts := []*Host{
		{
			Id:     "running_host_timeout",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(-time.Minute),
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "running_host_task",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "running_task",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "running_host_task_later_execution",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "restarted_task",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "running_host_task_same",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown:     time.Now().Add(time.Minute),
				TaskID:              "restarted_task",
				SpawnedByTask:       true,
				TaskExecutionNumber: 1,
			},
		},
		{
			Id:     "running_host_task_building",
			Status: evergreen.HostBuilding,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "running_task",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "running_host_build",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				BuildID:         "running_build",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "running_host_build_starting",
			Status: evergreen.HostStarting,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				BuildID:         "running_build",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "terminated_host_timeout",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(-time.Minute),
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "terminated_host_task",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "running_task",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "terminated_host_build",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				BuildID:         "running_build",
				SpawnedByTask:   true,
			},
		},
		{
			Id:     "host_not_spawned_by_task",
			Status: evergreen.HostRunning,
		},
	}
	for i := range hosts {
		require.NoError(hosts[i].Insert(ctx))
	}

	found, err := allHostsSpawnedByTasksTimedOut(ctx)
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal("running_host_timeout", found[0].Id)

	found, err = allHostsSpawnedByFinishedTasks(ctx)
	assert.NoError(err)
	assert.Len(found, 3)
	should := map[string]bool{
		"running_host_task":                 false,
		"running_host_task_building":        false,
		"running_host_task_later_execution": false,
	}
	for _, f := range found {
		should[f.Id] = true
	}
	for k, v := range should {
		assert.True(v, fmt.Sprintf("failed to find host %s", k))
	}

	found, err = allHostsSpawnedByFinishedBuilds(ctx)
	assert.NoError(err)
	assert.Len(found, 2)
	should = map[string]bool{
		"running_host_build":          false,
		"running_host_build_starting": false,
	}
	for _, f := range found {
		should[f.Id] = true
	}
	for k, v := range should {
		assert.True(v, fmt.Sprintf("failed to find host %s", k))
	}

	found, err = AllHostsSpawnedByTasksToTerminate(ctx)
	assert.NoError(err)
	assert.Len(found, 6)
	should = map[string]bool{
		"running_host_timeout":              false,
		"running_host_task":                 false,
		"running_host_task_building":        false,
		"running_host_build":                false,
		"running_host_build_starting":       false,
		"running_host_task_later_execution": false,
	}
	for _, f := range found {
		should[f.Id] = true
	}
	for k, v := range should {
		assert.True(v, fmt.Sprintf("failed to find host %s", k))
	}
}

func TestFindByFirstProvisioningAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(Collection))

	for _, h := range []Host{
		{
			Id:          "host1",
			Status:      evergreen.HostRunning,
			RunningTask: "task",
		},
		{
			Id:     "host2",
			Status: evergreen.HostStarting,
		},
		{
			Id:     "host3",
			Status: evergreen.HostProvisioning,
		},
		{
			Id:               "host4",
			Status:           evergreen.HostProvisioning,
			NeedsReprovision: ReprovisionToNew,
		},
	} {
		assert.NoError(h.Insert(ctx))
	}

	hosts, err := FindByProvisioning(ctx)
	require.NoError(err)
	require.Len(hosts, 1)
	assert.Equal("host3", hosts[0].Id)
}

func TestCountContainersRunningAtTime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	// time variables
	now := time.Now()
	startTimeBefore := now.Add(-10 * time.Minute)
	terminationTimeBefore := now.Add(-5 * time.Minute)
	startTimeAfter := now.Add(5 * time.Minute)
	terminationTimeAfter := now.Add(10 * time.Minute)

	parent := &Host{
		Id:            "parent",
		HasContainers: true,
	}

	containers := []Host{
		{
			Id:              "container1",
			ParentID:        "parent",
			StartTime:       startTimeBefore,
			TerminationTime: terminationTimeBefore,
		},
		{
			Id:              "container2",
			ParentID:        "parent",
			StartTime:       startTimeBefore,
			TerminationTime: terminationTimeAfter,
		},
		{
			Id:        "container3",
			ParentID:  "parent",
			StartTime: startTimeBefore,
		},
		{
			Id:        "container4",
			ParentID:  "parent",
			StartTime: startTimeAfter,
		},
	}
	for i := range containers {
		assert.NoError(containers[i].Insert(ctx))
	}

	count1, err := parent.CountContainersRunningAtTime(ctx, now.Add(-15*time.Minute))
	assert.NoError(err)
	assert.Equal(0, count1)

	count2, err := parent.CountContainersRunningAtTime(ctx, now)
	assert.NoError(err)
	assert.Equal(2, count2)

	count3, err := parent.CountContainersRunningAtTime(ctx, now.Add(15*time.Minute))
	assert.NoError(err)
	assert.Equal(2, count3)
}

func TestFindTerminatedHostsRunningTasksQuery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	t.Run("QueryExecutesProperly", func(t *testing.T) {
		hosts, err := FindTerminatedHostsRunningTasks(ctx)
		assert.NoError(t, err)
		assert.Len(t, hosts, 0)
	})
	t.Run("QueryFindsResults", func(t *testing.T) {
		h := Host{
			Id:          "bar",
			RunningTask: "foo",
			Status:      evergreen.HostTerminated,
		}
		assert.NoError(t, h.Insert(ctx))

		hosts, err := FindTerminatedHostsRunningTasks(ctx)
		assert.NoError(t, err)
		if assert.Len(t, hosts, 1) {
			assert.Equal(t, h.Id, hosts[0].Id)
		}
	})
}

func TestFindUphostParents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	h1 := Host{
		Id:            "h1",
		Status:        evergreen.HostRunning,
		HasContainers: true,
	}
	h2 := Host{
		Id:            "h2",
		Status:        evergreen.HostUninitialized,
		HasContainers: true,
		ContainerPoolSettings: &evergreen.ContainerPool{
			Distro:        "d1",
			Id:            "test-pool",
			MaxContainers: 100,
		},
	}
	h3 := Host{
		Id:            "h3",
		Status:        evergreen.HostRunning,
		HasContainers: true,
		ContainerPoolSettings: &evergreen.ContainerPool{
			Distro:        "d1",
			Id:            "test-pool",
			MaxContainers: 100,
		},
	}
	h4 := Host{
		Id:            "h4",
		Status:        evergreen.HostUninitialized,
		HasContainers: true,
	}
	h5 := Host{
		Id:       "h5",
		Status:   evergreen.HostUninitialized,
		ParentID: "h1",
	}

	assert.NoError(h1.Insert(ctx))
	assert.NoError(h2.Insert(ctx))
	assert.NoError(h3.Insert(ctx))
	assert.NoError(h4.Insert(ctx))
	assert.NoError(h5.Insert(ctx))

	uphostParents, err := findUphostParentsByContainerPool(ctx, "test-pool")
	assert.NoError(err)
	assert.Equal(2, len(uphostParents))
}

func TestRemoveStaleInitializing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.Clear(Collection))
	defer func() {
		assert.NoError(db.Clear(Collection))
	}()

	now := time.Now()
	distro1 := distro.Distro{Id: "distro1"}
	distro2 := distro.Distro{Id: "distro2"}

	hosts := []Host{
		{
			Id:           "host1",
			Distro:       distro1,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-1 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host2",
			Distro:       distro1,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host3",
			Distro:       distro1,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameStatic,
		},
		{
			Id:           "host4",
			Distro:       distro2,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host5",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host6",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host7",
			Distro:       distro2,
			Status:       evergreen.HostRunning,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host8",
			Distro:       distro2,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			SpawnOptions: SpawnOptions{SpawnedByTask: true},
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
	}

	for _, h := range hosts {
		require.NoError(h.Insert(ctx))
	}

	require.NoError(RemoveStaleInitializing(ctx, distro1.Id))

	findByDistroID := func(distroID string) ([]Host, error) {
		distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		return Find(ctx, bson.M{
			distroIDKey: distroID,
		})
	}

	distro1Hosts, err := findByDistroID(distro1.Id)
	require.NoError(err)
	require.Len(distro1Hosts, 4)
	ids := map[string]struct{}{}
	for _, h := range distro1Hosts {
		ids[h.Id] = struct{}{}
	}
	assert.Contains(ids, "host1")
	assert.Contains(ids, "host3")
	assert.Contains(ids, "host5")
	assert.Contains(ids, "host6")

	require.NoError(RemoveStaleInitializing(ctx, distro2.Id))

	distro2Hosts, err := findByDistroID(distro2.Id)
	require.NoError(err)
	require.Len(distro2Hosts, 2)
	ids = map[string]struct{}{}
	for _, h := range distro2Hosts {
		ids[h.Id] = struct{}{}
	}
	assert.Contains(ids, "host7")
	assert.Contains(ids, "host8")

	totalHosts, err := Count(ctx, All)
	require.NoError(err)
	assert.Equal(totalHosts, len(distro1Hosts)+len(distro2Hosts))
}

func TestMarkStaleBuildingAsFailed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	now := time.Now()
	distro1 := distro.Distro{Id: "distro1"}
	distro2 := distro.Distro{Id: "distro2"}

	hosts := []Host{
		{
			Id:           "host1",
			Distro:       distro1,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host2",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host3",
			Distro:       distro1,
			Status:       evergreen.HostStarting,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameStatic,
		},
		{
			Id:           "host4",
			Distro:       distro1,
			Status:       evergreen.HostBuildingFailed,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host5",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host6",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
			SpawnOptions: SpawnOptions{SpawnedByTask: true},
		},
		{
			Id:           "host7",
			Distro:       distro2,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     true,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host8",
			Distro:       distro2,
			Status:       evergreen.HostRunning,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host9",
			Distro:       distro2,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
		{
			Id:           "host10",
			Distro:       distro2,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     true,
			Provider:     evergreen.ProviderNameEc2Fleet,
		},
	}

	for _, h := range hosts {
		require.NoError(t, h.Insert(ctx))
	}

	require.NoError(t, MarkStaleBuildingAsFailed(ctx, distro1.Id))

	checkStatus := func(t *testing.T, h Host, expectedStatus string) {
		dbHost, err := FindOne(ctx, ById(h.Id))
		require.NoError(t, err)
		require.NotZero(t, dbHost)
		assert.Equal(t, dbHost.Status, expectedStatus)
	}

	for _, h := range append([]Host{hosts[0]}, hosts[2:]...) {
		checkStatus(t, h, h.Status)
	}
	checkStatus(t, hosts[1], evergreen.HostBuildingFailed)

	require.NoError(t, MarkStaleBuildingAsFailed(ctx, distro2.Id))

	checkStatus(t, hosts[6], evergreen.HostBuildingFailed)
	checkStatus(t, hosts[8], evergreen.HostBuildingFailed)
}

func TestNumNewParentsNeeded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameMock,
		ContainerPool: "test-pool",
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 2}
	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host4 := &Host{
		Id:                    "host4",
		Distro:                d,
		Status:                evergreen.HostUninitialized,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}

	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))

	existingParents, err := findUphostParentsByContainerPool(ctx, d.ContainerPool)
	assert.NoError(err)
	assert.Len(existingParents, 2)
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents(ctx)
	assert.NoError(err)
	assert.Len(existingContainers, 2)

	parentsParams := newParentsNeededParams{
		numExistingParents:    len(existingParents),
		numExistingContainers: len(existingContainers),
		numContainersNeeded:   4,
		maxContainers:         pool.MaxContainers,
	}
	num := numNewParentsNeeded(parentsParams)
	assert.Equal(1, num)
}

func TestNumNewParentsNeeded2(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameMock,
		ContainerPool: "test-pool",
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 3}

	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &Host{
		Id:       "host2",
		Distro:   d,
		Status:   evergreen.HostRunning,
		ParentID: "host1",
	}
	host3 := &Host{
		Id:       "host3",
		Distro:   d,
		Status:   evergreen.HostTerminated,
		ParentID: "host1",
	}

	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))

	existingParents, err := findUphostParentsByContainerPool(ctx, d.ContainerPool)
	assert.NoError(err)
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents(ctx)
	assert.NoError(err)

	parentsParams := newParentsNeededParams{
		numExistingParents:    len(existingParents),
		numExistingContainers: len(existingContainers),
		numContainersNeeded:   1,
		maxContainers:         pool.MaxContainers,
	}
	num := numNewParentsNeeded(parentsParams)
	assert.Equal(0, num)
}

func TestFindAvailableParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:            "distro",
		Provider:      evergreen.ProviderNameMock,
		ContainerPool: "test-pool",
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}

	pool := &evergreen.ContainerPool{Distro: "parent-distro", Id: "test-pool", MaxContainers: 2}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &Host{
		Id:                    "host2",
		Distro:                distro.Distro{Id: "parent-distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &Host{
		Id:          "host3",
		Distro:      d,
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      d,
		Status:      evergreen.HostRunning,
		ParentID:    "host2",
		RunningTask: "task2",
	}
	task1 := task.Task{
		Id: "task1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		},
		BuildVariant: "bv1",
		StartTime:    time.Now(),
	}
	task2 := task.Task{
		Id: "task2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		},
		BuildVariant: "bv1",
		StartTime:    time.Now(),
	}
	assert.NoError(d.Insert(ctx))
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(task1.Insert())
	assert.NoError(task2.Insert())

	availableParent, err := GetContainersOnParents(ctx, d)
	assert.NoError(err)

	assert.Equal(2, len(availableParent))
}

func TestFindNoAvailableParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:       "distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 1}
	durationOne := 20 * time.Minute
	durationTwo := 30 * time.Minute

	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &Host{
		Id:                    "host2",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &Host{
		Id:          "host3",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	host4 := &Host{
		Id:          "host4",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
		Status:      evergreen.HostRunning,
		ParentID:    "host2",
		RunningTask: "task2",
	}
	task1 := task.Task{
		Id: "task1",
		DurationPrediction: util.CachedDurationValue{
			Value: durationOne,
		}, BuildVariant: "bv1",
		StartTime: time.Now(),
	}
	task2 := task.Task{
		Id: "task2",
		DurationPrediction: util.CachedDurationValue{
			Value: durationTwo,
		}, BuildVariant: "bv1",
		StartTime: time.Now(),
	}
	assert.NoError(d.Insert(ctx))
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))
	assert.NoError(host4.Insert(ctx))
	assert.NoError(task1.Insert())
	assert.NoError(task2.Insert())

	availableParent, err := GetContainersOnParents(ctx, d)
	assert.NoError(err)
	assert.Equal(0, len(availableParent))
}

func TestGetNumNewParentsAndHostsToSpawn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:       "distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 3,
		},
	}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 1}

	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host2 := &Host{
		Id:                    "host2",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostRunning,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	host3 := &Host{
		Id:          "host3",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
		Status:      evergreen.HostRunning,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	assert.NoError(d.Insert(ctx))
	assert.NoError(host1.Insert(ctx))
	assert.NoError(host2.Insert(ctx))
	assert.NoError(host3.Insert(ctx))

	parents, hosts, err := getNumNewParentsAndHostsToSpawn(ctx, pool, 3, false)
	assert.NoError(err)
	assert.Equal(1, parents) // need two parents, but can only spawn 1
	assert.Equal(2, hosts)

	parents, hosts, err = getNumNewParentsAndHostsToSpawn(ctx, pool, 3, true)
	assert.NoError(err)
	assert.Equal(2, parents)
	assert.Equal(3, hosts)
}

func TestGetNumNewParentsWithInitializingParentAndHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert := assert.New(t)
	require.NoError(t, db.ClearCollections(Collection, distro.Collection, task.Collection))

	d := distro.Distro{
		Id:       "distro",
		Provider: evergreen.ProviderNameMock,
		HostAllocatorSettings: distro.HostAllocatorSettings{
			MaximumHosts: 2,
		},
	}
	pool := &evergreen.ContainerPool{Distro: "distro", Id: "test-pool", MaxContainers: 2}

	host1 := &Host{
		Id:                    "host1",
		Host:                  "host",
		User:                  "user",
		Distro:                distro.Distro{Id: "distro"},
		Status:                evergreen.HostUninitialized,
		HasContainers:         true,
		ContainerPoolSettings: pool,
	}
	container := &Host{
		Id:          "container",
		Distro:      distro.Distro{Id: "distro", ContainerPool: "test-pool"},
		Status:      evergreen.HostUninitialized,
		ParentID:    "host1",
		RunningTask: "task1",
	}
	assert.NoError(d.Insert(ctx))
	assert.NoError(host1.Insert(ctx))
	assert.NoError(container.Insert(ctx))

	parents, hosts, err := getNumNewParentsAndHostsToSpawn(ctx, pool, 4, false)
	assert.NoError(err)
	assert.Equal(1, parents) // need two parents, but can only spawn 1
	assert.Equal(3, hosts)   // should consider the uninitialized container as taking up capacity

	parents, hosts, err = getNumNewParentsAndHostsToSpawn(ctx, pool, 4, true)
	assert.NoError(err)
	assert.Equal(2, parents)
	assert.Equal(4, hosts)
}

func TestAddTags(t *testing.T) {
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
			{Key: "key-1", Value: "val-1", CanBeModified: true},
			{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	tagsToAdd := []Tag{
		{Key: "key-fixed", Value: "val-new", CanBeModified: false},
		{Key: "key-2", Value: "val-new", CanBeModified: true},
		{Key: "key-3", Value: "val-3", CanBeModified: true},
	}
	h.AddTags(tagsToAdd)
	assert.Equal(t, []Tag{
		{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
		{Key: "key-1", Value: "val-1", CanBeModified: true},
		{Key: "key-2", Value: "val-new", CanBeModified: true},
		{Key: "key-3", Value: "val-3", CanBeModified: true},
	}, h.InstanceTags)
}

func TestDeleteTags(t *testing.T) {
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
			{Key: "key-1", Value: "val-1", CanBeModified: true},
			{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	tagsToDelete := []string{"key-fixed", "key-1"}
	h.DeleteTags(tagsToDelete)
	assert.Equal(t, []Tag{
		{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
		{Key: "key-2", Value: "val-2", CanBeModified: true},
	}, h.InstanceTags)
}

func TestSetTags(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			{Key: "key-1", Value: "val-1", CanBeModified: true},
			{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	assert.NoError(t, h.Insert(ctx))
	h.InstanceTags = []Tag{
		{Key: "key-3", Value: "val-3", CanBeModified: true},
	}
	assert.NoError(t, h.SetTags(ctx))
	foundHost, err := FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	assert.Equal(t, h.InstanceTags, foundHost.InstanceTags)
}

func TestMakeHostTags(t *testing.T) {
	t.Run("OK", func(t *testing.T) {
		tagSlice := []string{"key1=value1", "key2=value2"}
		tags, err := MakeHostTags(tagSlice)
		require.NoError(t, err)

		assert.Contains(t, tags, Tag{
			Key:           "key1",
			Value:         "value1",
			CanBeModified: true,
		})

		assert.Contains(t, tags, Tag{
			Key:           "key2",
			Value:         "value2",
			CanBeModified: true,
		})
	})
	t.Run("ParsingError", func(t *testing.T) {
		badTag := "incorrect"
		tagSlice := []string{"key1=value1", badTag}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("parsing tag '%s'", badTag))
	})
	t.Run("LongKey", func(t *testing.T) {
		badKey := strings.Repeat("a", 129)
		tagSlice := []string{"key1=value", fmt.Sprintf("%s=value2", badKey)}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("key '%s' is longer than maximum limit of 128 characters", badKey))
	})
	t.Run("LongValue", func(t *testing.T) {
		badValue := strings.Repeat("a", 257)
		tagSlice := []string{"key1=value2", fmt.Sprintf("key2=%s", badValue)}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("value '%s' is longer than maximum limit of 256 characters", badValue))
	})
	t.Run("BadPrefix", func(t *testing.T) {
		badPrefix := "aws:"
		tagSlice := []string{"key1=value1", fmt.Sprintf("%skey2=value2", badPrefix)}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("illegal tag prefix '%s'", badPrefix))
	})
}

func TestSetInstanceType(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id:           "id",
		InstanceType: "old-instance-type",
	}
	assert.NoError(t, h.Insert(ctx))
	newInstanceType := "new-instance-type"
	assert.NoError(t, h.SetInstanceType(ctx, newInstanceType))
	foundHost, err := FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	assert.Equal(t, newInstanceType, foundHost.InstanceType)
}

func TestAggregateSpawnhostData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection, VolumesCollection))
	hosts := []Host{
		{
			Id:           "host-1",
			Status:       evergreen.HostRunning,
			InstanceType: "small",
			StartedBy:    "me",
			UserHost:     true,
			NoExpiration: true,
		},
		{
			Id:           "host-2",
			Status:       evergreen.HostRunning,
			InstanceType: "small",
			StartedBy:    "me",
			UserHost:     true,
			NoExpiration: false,
		},
		{
			Id:           "host-3",
			Status:       evergreen.HostStopped,
			InstanceType: "large",
			StartedBy:    "you",
			UserHost:     true,
			NoExpiration: false,
		},
		{
			Id:           "host-4",
			Status:       evergreen.HostRunning,
			InstanceType: "medium",
			StartedBy:    "her",
			UserHost:     true,
			NoExpiration: true,
		},
		{
			Id:        "host-5",
			Status:    evergreen.HostStarting,
			StartedBy: "no-one",
		},
		{
			Id:           "host-6",
			Status:       evergreen.HostTerminated,
			InstanceType: "tiny",
			StartedBy:    "doesnt-matter",
			UserHost:     true,
		},
	}
	for _, h := range hosts {
		assert.NoError(t, h.Insert(ctx))
	}

	volumes := []Volume{
		{
			ID:        "v1",
			CreatedBy: "me",
			Size:      100,
		},
		{
			ID:        "v2",
			CreatedBy: "me",
			Size:      12,
		},
		{
			ID:        "v3",
			CreatedBy: "you",
			Size:      300,
		},
	}
	for _, v := range volumes {
		assert.NoError(t, v.Insert())
	}

	res, err := AggregateSpawnhostData(ctx)
	assert.NoError(t, err)
	require.NotNil(t, res)
	assert.Equal(t, 3, res.NumUsersWithHosts)
	assert.Equal(t, 4, res.TotalHosts)
	assert.Equal(t, 1, res.TotalStoppedHosts)
	assert.EqualValues(t, 2, res.TotalUnexpirableHosts)
	assert.Equal(t, 2, res.NumUsersWithVolumes)
	assert.Equal(t, 412, res.TotalVolumeSize)
	assert.Equal(t, 3, res.TotalVolumes)
	assert.NotNil(t, res.InstanceTypes)
	assert.Len(t, res.InstanceTypes, 3)
	assert.Equal(t, res.InstanceTypes["small"], 2)
	assert.Equal(t, res.InstanceTypes["medium"], 1)
	assert.Equal(t, res.InstanceTypes["large"], 1)
	assert.Equal(t, res.InstanceTypes["tiny"], 0)
}

func TestCountSpawnhostsWithNoExpirationByUser(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	hosts := []Host{
		{
			Id:           "host-1",
			Status:       evergreen.HostRunning,
			StartedBy:    "user-1",
			NoExpiration: true,
		},
		{
			Id:           "host-2",
			Status:       evergreen.HostRunning,
			StartedBy:    "user-1",
			NoExpiration: false,
		},
		{
			Id:           "host-3",
			Status:       evergreen.HostTerminated,
			StartedBy:    "user-1",
			NoExpiration: true,
		},
		{
			Id:           "host-4",
			Status:       evergreen.HostRunning,
			StartedBy:    "user-2",
			NoExpiration: true,
		},
		{
			Id:           "host-5",
			Status:       evergreen.HostStarting,
			StartedBy:    "user-2",
			NoExpiration: true,
		},
	}
	for _, h := range hosts {
		assert.NoError(t, h.Insert(ctx))
	}
	count, err := CountSpawnhostsWithNoExpirationByUser(ctx, "user-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	count, err = CountSpawnhostsWithNoExpirationByUser(ctx, "user-2")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	count, err = CountSpawnhostsWithNoExpirationByUser(ctx, "user-3")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestFindSpawnhostsWithNoExpirationToExtend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	hosts := []Host{
		{
			Id:             "host-1",
			UserHost:       true,
			Status:         evergreen.HostRunning,
			NoExpiration:   true,
			ExpirationTime: time.Now(),
		},
		{
			Id:             "host-2",
			UserHost:       true,
			Status:         evergreen.HostRunning,
			NoExpiration:   false,
			ExpirationTime: time.Now(),
		},
		{
			Id:             "host-3",
			UserHost:       true,
			Status:         evergreen.HostTerminated,
			NoExpiration:   true,
			ExpirationTime: time.Now(),
		},
		{
			Id:             "host-4",
			UserHost:       true,
			Status:         evergreen.HostRunning,
			NoExpiration:   true,
			ExpirationTime: time.Now().AddDate(1, 0, 0),
		},
		{
			Id:             "host-5",
			UserHost:       false,
			Status:         evergreen.HostRunning,
			NoExpiration:   true,
			ExpirationTime: time.Now(),
		},
	}
	for _, h := range hosts {
		assert.NoError(t, h.Insert(ctx))
	}

	foundHosts, err := FindSpawnhostsWithNoExpirationToExtend(ctx)
	assert.NoError(t, err)
	assert.Len(t, foundHosts, 1)
	assert.Equal(t, "host-1", foundHosts[0].Id)
}

func TestAddVolumeToHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id: "host-1",
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
		},
	}
	assert.NoError(t, h.Insert(ctx))

	newAttachment := &VolumeAttachment{
		VolumeID:   "volume-2",
		DeviceName: "device-2",
	}
	assert.NoError(t, h.AddVolumeToHost(ctx, newAttachment))
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
		{
			VolumeID:   "volume-2",
			DeviceName: "device-2",
		},
	}, h.Volumes)
	foundHost, err := FindOneId(ctx, "host-1")
	assert.NoError(t, err)
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
		{
			VolumeID:   "volume-2",
			DeviceName: "device-2",
		},
	}, foundHost.Volumes)
}

func TestUnsetHomeVolume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id:           "host-1",
		HomeVolumeID: "volume-1",
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
		},
	}
	assert.NoError(t, h.Insert(ctx))
	assert.NoError(t, h.UnsetHomeVolume(ctx))
	assert.Equal(t, "", h.HomeVolumeID)
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, h.Volumes)
	foundHost, err := FindOneId(ctx, "host-1")
	assert.NoError(t, err)
	assert.Equal(t, "", foundHost.HomeVolumeID)
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, foundHost.Volumes)
}

func TestRemoveVolumeFromHost(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id: "host-1",
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
			{
				VolumeID:   "volume-2",
				DeviceName: "device-2",
			},
		},
	}
	assert.NoError(t, h.Insert(ctx))
	assert.NoError(t, h.RemoveVolumeFromHost(ctx, "volume-2"))
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, h.Volumes)
	foundHost, err := FindOneId(ctx, "host-1")
	assert.NoError(t, err)
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, foundHost.Volumes)
}

func TestFindHostWithVolume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id:       "host-1",
		UserHost: true,
		Status:   evergreen.HostRunning,
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
		},
	}
	assert.NoError(t, h.Insert(ctx))
	foundHost, err := FindHostWithVolume(ctx, "volume-1")
	assert.NoError(t, err)
	assert.NotNil(t, foundHost)
	foundHost, err = FindHostWithVolume(ctx, "volume-2")
	assert.NoError(t, err)
	assert.Nil(t, foundHost)
}

func TestFindHostsInRange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	distroEast := birch.NewDocument(birch.EC.String("region", "us-east-1"))
	distroWest := distroEast.Copy().Set(birch.EC.String("region", "us-west-1"))
	hosts := []Host{
		{
			Id:           "h0",
			Status:       evergreen.HostTerminated,
			CreationTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock, ProviderSettingsList: []*birch.Document{distroEast}},
		},
		{
			Id:           "h1",
			Status:       evergreen.HostRunning,
			CreationTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
			Distro:       distro.Distro{Id: "ubuntu-1604", Provider: evergreen.ProviderNameMock, ProviderSettingsList: []*birch.Document{distroWest}},
		},
		{
			Id:           "h2",
			Status:       evergreen.HostRunning,
			CreationTime: time.Date(2009, time.December, 10, 23, 0, 0, 0, time.UTC),
			Distro:       distro.Distro{Id: "ubuntu-1804", Provider: evergreen.ProviderNameMock, ProviderSettingsList: []*birch.Document{distroWest}},
		},
	}
	for _, h := range hosts {
		require.NoError(t, h.Insert(ctx))
	}

	filteredHosts, err := FindHostsInRange(ctx, HostsInRangeParams{Status: evergreen.HostTerminated})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 1)
	assert.Equal(t, "h0", filteredHosts[0].Id)

	filteredHosts, err = FindHostsInRange(ctx, HostsInRangeParams{Distro: "ubuntu-1604"})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 1)
	assert.Equal(t, "h1", filteredHosts[0].Id)

	filteredHosts, err = FindHostsInRange(ctx, HostsInRangeParams{CreatedAfter: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 1)
	assert.Equal(t, "h2", filteredHosts[0].Id)

	filteredHosts, err = FindHostsInRange(ctx, HostsInRangeParams{Region: "us-east-1", Status: evergreen.HostTerminated})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 1)

	filteredHosts, err = FindHostsInRange(ctx, HostsInRangeParams{Region: "us-west-1"})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 2)

	filteredHosts, err = FindHostsInRange(ctx, HostsInRangeParams{Region: "us-west-1", Distro: "ubuntu-1604"})
	assert.NoError(t, err)
	assert.Len(t, filteredHosts, 1)
	assert.Equal(t, "h1", filteredHosts[0].Id)
}

func TestRemoveAndReplace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	require.NoError(t, db.Clear(Collection))

	// removing a nonexistent host errors
	assert.Error(t, RemoveStrict(ctx, env, "asdf"))

	// replacing an existing host works
	h := Host{
		Id:     "bar",
		Status: evergreen.HostUninitialized,
	}
	assert.NoError(t, h.Insert(ctx))

	h.DockerOptions.Command = "hello world"
	assert.NoError(t, h.Replace(ctx))
	dbHost, err := FindOneId(ctx, h.Id)
	assert.NoError(t, err)
	assert.Equal(t, evergreen.HostUninitialized, dbHost.Status)
	assert.Equal(t, "hello world", dbHost.DockerOptions.Command)

	// replacing a nonexisting host will just insert
	h2 := Host{
		Id:     "host2",
		Status: evergreen.HostRunning,
	}
	assert.NoError(t, h2.Replace(ctx))
	dbHost, err = FindOneId(ctx, h2.Id)
	assert.NoError(t, err)
	assert.NotNil(t, dbHost)
}

func TestFindStaticNeedsNewSSHKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	keyName := "key"
	for testName, testCase := range map[string]func(t *testing.T, settings *evergreen.Settings, h *Host){
		"IgnoresHostsWithMatchingKeys": func(t *testing.T, settings *evergreen.Settings, h *Host) {
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindStaticNeedsNewSSHKeys(ctx, settings)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"FindsHostsMissingAllKeys": func(t *testing.T, settings *evergreen.Settings, h *Host) {
			h.SSHKeyNames = []string{}
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindStaticNeedsNewSSHKeys(ctx, settings)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"FindsHostsMissingSubsetOfKeys": func(t *testing.T, settings *evergreen.Settings, h *Host) {
			require.NoError(t, h.Insert(ctx))

			newKeyName := "new_key"
			settings.SSHKeyPairs = append(settings.SSHKeyPairs, evergreen.SSHKeyPair{
				Name:    newKeyName,
				Public:  "new_public",
				Private: "new_private",
			})

			hosts, err := FindStaticNeedsNewSSHKeys(ctx, settings)
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"IgnoresNonstaticHosts": func(t *testing.T, settings *evergreen.Settings, h *Host) {
			h.SSHKeyNames = []string{}
			h.Provider = evergreen.ProviderNameMock
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindStaticNeedsNewSSHKeys(ctx, settings)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresHostsWithExtraKeys": func(t *testing.T, settings *evergreen.Settings, h *Host) {
			h.SSHKeyNames = append(h.SSHKeyNames, "other_key")
			require.NoError(t, h.Insert(ctx))

			hosts, err := FindStaticNeedsNewSSHKeys(ctx, settings)
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()

			settings := &evergreen.Settings{
				SSHKeyDirectory: "/ssh_key_directory",
				SSHKeyPairs: []evergreen.SSHKeyPair{
					{
						Name:    keyName,
						Public:  "public",
						Private: "private",
					},
				},
			}

			h := &Host{
				Id:          "id",
				Provider:    evergreen.ProviderNameStatic,
				Status:      evergreen.HostRunning,
				SSHKeyNames: []string{keyName},
			}

			testCase(t, settings, h)
		})
	}
}

func TestSetNewSSHKeys(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	h := &Host{
		Id: "foo",
	}
	assert.Error(t, h.AddSSHKeyName(ctx, "foo"))
	assert.Empty(t, h.SSHKeyNames)

	require.NoError(t, h.Insert(ctx))
	require.NoError(t, h.AddSSHKeyName(ctx, "foo"))
	assert.Equal(t, []string{"foo"}, h.SSHKeyNames)

	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.Equal(t, []string{"foo"}, dbHost.SSHKeyNames)

	require.NoError(t, h.AddSSHKeyName(ctx, "bar"))
	assert.Subset(t, []string{"foo", "bar"}, h.SSHKeyNames)
	assert.Subset(t, h.SSHKeyNames, []string{"foo", "bar"})

	dbHost, err = FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.Subset(t, []string{"foo", "bar"}, dbHost.SSHKeyNames)
	assert.Subset(t, dbHost.SSHKeyNames, []string{"foo", "bar"})
}

func TestUpdateCachedDistroProviderSettings(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))

	h := &Host{
		Id: "h0",
		Distro: distro.Distro{
			Id: "d0",
			ProviderSettingsList: []*birch.Document{
				birch.NewDocument(
					birch.EC.String("subnet_id", "subnet-123456"),
					birch.EC.String("region", evergreen.DefaultEC2Region),
				),
			},
		},
	}
	require.NoError(t, h.Insert(ctx))

	newProviderSettings := []*birch.Document{
		birch.NewDocument(
			birch.EC.String("subnet_id", "new_subnet"),
			birch.EC.String("region", evergreen.DefaultEC2Region),
		),
	}
	assert.NoError(t, h.UpdateCachedDistroProviderSettings(ctx, newProviderSettings))

	// updated in memory
	assert.Equal(t, "new_subnet", h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValue())

	h, err := FindOneId(ctx, "h0")
	require.NoError(t, err)
	require.NotNil(t, h)

	// updated in the db
	assert.Equal(t, "new_subnet", h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValue())
}

func TestPartitionParents(t *testing.T) {
	assert := assert.New(t)
	const distroId = "match"

	parents := []ContainersOnParents{
		{ParentHost: Host{Id: "1"}, Containers: []Host{{}, {}, {Distro: distro.Distro{Id: distroId}}}},
		{ParentHost: Host{Id: "2"}, Containers: []Host{{}, {Distro: distro.Distro{Id: "foo"}}}},
		{ParentHost: Host{Id: "3"}, Containers: []Host{{Distro: distro.Distro{Id: distroId}}, {Distro: distro.Distro{Id: "foo"}}}},
		{ParentHost: Host{Id: "4"}, Containers: []Host{{Distro: distro.Distro{Id: "foo"}}}},
	}
	matched, notMatched := partitionParents(parents, distroId, defaultMaxImagesPerParent)
	assert.Len(matched, 2)
	assert.Len(notMatched, 2)
	assert.Equal("1", matched[0].ParentHost.Id)
	assert.Equal("3", matched[1].ParentHost.Id)
	assert.Equal("2", notMatched[0].ParentHost.Id)
	assert.Equal("4", notMatched[1].ParentHost.Id)

	parents = []ContainersOnParents{
		{ParentHost: Host{Id: "1"}, Containers: []Host{{Distro: distro.Distro{Id: "1"}}, {Distro: distro.Distro{Id: "2"}}, {Distro: distro.Distro{Id: "3"}}}},
	}
	matched, notMatched = partitionParents(parents, distroId, defaultMaxImagesPerParent)
	assert.Len(matched, 0)
	assert.Len(notMatched, 0)
}

func TestCountVirtualWorkstationsByDistro(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	for i := 0; i < 100; i++ {
		h := &Host{
			Id:           fmt.Sprintf("%d", i),
			Status:       evergreen.HostTerminated,
			InstanceType: "foo",
		}
		if i%3 == 0 {
			h.Status = evergreen.HostRunning
		}
		if i%5 == 0 {
			h.IsVirtualWorkstation = true
		}
		if i%11 == 0 {
			h.InstanceType = "bar"
		}
		require.NoError(t, h.Insert(ctx))
	}
	count, err := CountVirtualWorkstationsByInstanceType(ctx)
	require.NoError(t, err)
	for _, counter := range count {
		require.NotEmpty(t, counter.InstanceType)
		if counter.InstanceType == "foo" {
			require.Equal(t, 6, counter.Count)
		}
		if counter.InstanceType == "bar" {
			require.Equal(t, 1, counter.Count)
		}
	}
}

func TestGetAMI(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id: "myEC2Host",
		Distro: distro.Distro{
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("ami", "ami"),
				birch.EC.String("key_name", "key"),
				birch.EC.String("instance_type", "instance"),
				birch.EC.String("aws_access_key_id", "key_id"),
				birch.EC.Double("bid_price", 0.001),
				birch.EC.SliceString("security_group_ids", []string{"abcdef"}),
			)},
			Provider: evergreen.ProviderNameEc2OnDemand,
		},
	}
	h2 := &Host{
		Id:       "myOtherHost",
		Provider: evergreen.ProviderNameMock,
	}
	assert.Equal(t, h.GetAMI(), "ami")
	assert.Equal(t, h2.GetAMI(), "")
}

func TestGetSubnetID(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id: "myEC2Host",
		Distro: distro.Distro{
			ProviderSettingsList: []*birch.Document{birch.NewDocument(
				birch.EC.String("subnet_id", "swish"),
			)},
			Provider: evergreen.ProviderNameEc2OnDemand,
		},
	}
	h2 := &Host{
		Id:       "myOtherHost",
		Provider: evergreen.ProviderNameMock,
	}
	assert.Equal(t, h.GetSubnetID(), "swish")
	assert.Equal(t, h2.GetSubnetID(), "")
}

func TestUnsafeReplace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, old, replacement Host){
		"SuccessfullySwapsHosts": func(ctx context.Context, t *testing.T, env evergreen.Environment, old, replacement Host) {
			require.NoError(t, old.Insert(ctx))

			require.NoError(t, UnsafeReplace(ctx, env, old.Id, &replacement))

			found, err := FindOne(ctx, ById(old.Id))
			assert.NoError(t, err)
			assert.Zero(t, found)

			found, err = FindOne(ctx, ById(replacement.Id))
			assert.NoError(t, err)
			require.NotZero(t, found)
			assert.Equal(t, *found, replacement)
		},
		"SucceedsIfNewHostIsIdenticalToOldHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, h, _ Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, UnsafeReplace(ctx, env, h.Id, &h))

			found, err := FindOne(ctx, ById(h.Id))
			assert.NoError(t, err)
			require.NotZero(t, found)
			assert.Equal(t, *found, h)
		},
		"FailsIfOldHostIsMissing": func(ctx context.Context, t *testing.T, env evergreen.Environment, old, replacement Host) {
			require.Error(t, UnsafeReplace(ctx, env, old.Id, &replacement))

			h, err := FindOne(ctx, ById(old.Id))
			assert.NoError(t, err)
			assert.Zero(t, h)

			h, err = FindOne(ctx, ById(replacement.Id))
			assert.NoError(t, err)
			assert.Zero(t, h)
		},
		"FailsIfNewHostAlreadyExists": func(ctx context.Context, t *testing.T, env evergreen.Environment, old, replacement Host) {
			require.NoError(t, replacement.Insert(ctx))

			require.Error(t, UnsafeReplace(ctx, env, old.Id, &replacement))

			found, err := FindOne(ctx, ById(old.Id))
			assert.NoError(t, err)
			assert.Zero(t, found)

			found, err = FindOne(ctx, ById(replacement.Id))
			assert.NoError(t, err)
			require.NotZero(t, found)
			assert.Equal(t, *found, replacement)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			old := Host{
				Id:     "old",
				Status: evergreen.HostBuilding,
			}
			replacement := Host{
				Id:     "replacement",
				Status: evergreen.HostStarting,
			}
			tCase(ctx, t, env, old, replacement)
		})
	}
}

type FindHostsSuite struct {
	setup func(*FindHostsSuite)
	suite.Suite
}

const testUser = "user1"

func (*FindHostsSuite) hosts() []Host {
	return []Host{
		{
			Id:        "host1",
			StartedBy: testUser,
			Distro: distro.Distro{
				Id:      "distro1",
				Aliases: []string{"alias125"},
				Arch:    evergreen.ArchLinuxAmd64,
			},
			Status:         evergreen.HostRunning,
			ExpirationTime: time.Now().Add(time.Hour),
			Secret:         "abcdef",
			IP:             "ip1",
		}, {
			Id:        "host2",
			StartedBy: "user2",
			Distro: distro.Distro{
				Id:      "distro2",
				Aliases: []string{"alias125"},
			},
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip2",
		}, {
			Id:             "host3",
			StartedBy:      "user3",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip3",
		}, {
			Id:             "host4",
			StartedBy:      "user4",
			Status:         evergreen.HostTerminated,
			ExpirationTime: time.Now().Add(time.Hour),
			IP:             "ip4",
		}, {
			Id:        "host5",
			StartedBy: evergreen.User,
			Status:    evergreen.HostRunning,
			Distro: distro.Distro{
				Id:      "distro5",
				Aliases: []string{"alias125"},
			},
			IP: "ip5",
		},
	}
}

func TestFindHostsSuite(t *testing.T) {
	s := new(FindHostsSuite)

	s.setup = func(s *FindHostsSuite) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		s.NoError(db.ClearCollections(user.Collection, Collection, evergreen.ScopeCollection, evergreen.RoleCollection))
		require.NoError(t, db.CreateCollections(evergreen.ScopeCollection))

		hosts := s.hosts()
		for _, h := range hosts {
			s.Require().NoError(h.Insert(ctx))
		}

		users := []string{testUser, "user2", "user3", "user4"}

		for _, id := range users {
			user := &user.DBUser{
				Id: id,
			}
			s.NoError(user.Insert())
		}
		root := user.DBUser{
			Id:          "root",
			SystemRoles: []string{"root"},
		}
		s.NoError(root.Insert())
		rm := evergreen.GetEnvironment().RoleManager()
		s.NoError(rm.AddScope(gimlet.Scope{
			ID:        "root",
			Resources: []string{"distro2", "distro5"},
			Type:      evergreen.DistroResourceType,
		}))
		s.NoError(rm.UpdateRole(gimlet.Role{
			ID:    "root",
			Scope: "root",
			Permissions: gimlet.Permissions{
				evergreen.PermissionHosts: evergreen.HostsEdit.Value,
			},
		}))
	}

	suite.Run(t, s)
}

func (s *FindHostsSuite) SetupTest() {
	s.NotNil(s.setup)
	s.setup(s)
}

func (s *FindHostsSuite) TestFindById() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, ok := FindOneId(ctx, "host1")
	s.NoError(ok)
	s.NotNil(h1)
	s.Equal("host1", h1.Id)

	h2, ok := FindOneId(ctx, "host2")
	s.NoError(ok)
	s.NotNil(h2)
	s.Equal("host2", h2.Id)
}

func (s *FindHostsSuite) TestFindByIdFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, ok := FindOneId(ctx, "nonexistent")
	s.NoError(ok)
	s.Nil(h)
}

func (s *FindHostsSuite) TestFindByIP() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, ok := FindOne(ctx, ByIPAndRunning("ip1"))
	s.NoError(ok)
	s.NotNil(h1)
	s.Equal("host1", h1.Id)
	s.Equal("ip1", h1.IP)
}

func (s *FindHostsSuite) TestFindByIPFail() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// terminated host
	h1, ok := FindOne(ctx, ByIPAndRunning("ip2"))
	s.NoError(ok)
	s.Nil(h1)

	h2, ok := FindOne(ctx, ByIPAndRunning("nonexistent"))
	s.NoError(ok)
	s.Nil(h2)
}

func (s *FindHostsSuite) TestFindHostsByDistro() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts, err := Find(ctx, ByDistroIDsOrAliasesRunning("distro5"))
	s.Require().NoError(err)
	s.Require().Len(hosts, 1)
	s.Equal("host5", hosts[0].Id)

	hosts, err = Find(ctx, ByDistroIDsOrAliasesRunning("alias125"))
	s.Require().NoError(err)
	s.Require().Len(hosts, 2)
	var host1Found, host5Found bool
	for _, h := range hosts {
		if h.Id == "host1" {
			host1Found = true
		}
		if h.Id == "host5" {
			host5Found = true
		}
	}
	s.True(host1Found)
	s.True(host5Found)
}

func (s *FindHostsSuite) TestFindByUser() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts, err := GetHostsByFromIDWithStatus(ctx, "", "", testUser, 100)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		s.Equal(testUser, h.StartedBy)
	}
}

func (s *FindHostsSuite) TestStatusFiltering() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts, err := GetHostsByFromIDWithStatus(ctx, "", "", "", 100)
	s.NoError(err)
	s.NotNil(hosts)
	for _, h := range hosts {
		statusFound := false
		for _, status := range evergreen.UpHostStatus {
			if h.Status == status {
				statusFound = true
			}
		}
		s.True(statusFound)
	}
}

func (s *FindHostsSuite) TestLimit() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hosts, err := GetHostsByFromIDWithStatus(ctx, "", evergreen.HostTerminated, "", 2)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(2, len(hosts))
	s.Equal("host2", hosts[0].Id)
	s.Equal("host3", hosts[1].Id)

	hosts, err = GetHostsByFromIDWithStatus(ctx, "", evergreen.HostTerminated, "", 3)
	s.NoError(err)
	s.NotNil(hosts)
	s.Equal(3, len(hosts))
}

func (s *FindHostsSuite) TestSetHostStatus() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := FindOneId(ctx, "host1")
	s.NoError(err)
	s.NoError(h.SetStatus(ctx, evergreen.HostTerminated, evergreen.User, fmt.Sprintf("changed by %s from API", evergreen.User)))

	for i := 1; i < 5; i++ {
		h, err := FindOneId(ctx, fmt.Sprintf("host%d", i))
		s.NoError(err)
		s.Equal(evergreen.HostTerminated, h.Status)
	}
}

func (s *FindHostsSuite) TestExtendHostExpiration() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h, err := FindOneId(ctx, "host1")
	s.NoError(err)
	expectedTime := h.ExpirationTime.Add(5 * time.Hour)
	s.NoError(h.SetExpirationTime(ctx, expectedTime))

	hCheck, err := FindOneId(ctx, "host1")
	s.Equal(expectedTime, hCheck.ExpirationTime)
	s.NoError(err)
}

func setupHostTerminationQueryIndex(t *testing.T) {
	require.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{
		Keys: StatusIndex,
	}))
}

func TestFindHostsToTerminate(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.DropCollections(Collection))
	}()
	setupHostTerminationQueryIndex(t)

	for tName, tCase := range map[string]func(t *testing.T){
		"IncludesSpawnHostsThatExceedExpirationTime": func(t *testing.T) {
			h := &Host{
				Id:             "h1",
				StartedBy:      "normal-user",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-10 * time.Minute),
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
		"IncludesSpawnHostsThatHaveNotExceededExpirationTime": func(t *testing.T) {
			h := &Host{
				Id:             "h1",
				StartedBy:      "normal-user",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(time.Hour),
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			assert.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IncludesDecommissionedHosts": func(t *testing.T) {
			h := &Host{
				Provider: evergreen.ProviderNameMock,
				Id:       "h3",
				Status:   evergreen.HostDecommissioned,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
		"IgnoresQuarantinedHosts": func(t *testing.T) {
			h := &Host{
				Id:       "id",
				Provider: evergreen.ProviderNameMock,
				Status:   evergreen.HostQuarantined,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			assert.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IncludesHostsThatFailedToBuild": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Status:       evergreen.HostBuildingFailed,
				Provider:     evergreen.ProviderNameMock,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
		"IncludesHostsThatFailedToProvision": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Status:       evergreen.HostProvisionFailed,
				Provider:     evergreen.ProviderNameMock,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
		"IgnoresAlreadyTerminatedHosts": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				Provider:     evergreen.ProviderNameMock,
				Status:       evergreen.HostTerminated,
				CreationTime: time.Now().Add(-time.Minute * 60),
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			assert.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IgnoresHostsThatAreAlreadyProvisioned": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				StartedBy:    evergreen.User,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-time.Minute * 60),
				Provisioned:  true,
				Status:       evergreen.HostRunning,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			assert.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IgnoresHostsThatHaveNotExceededProvisioningDeadline": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				StartedBy:    evergreen.User,
				Provider:     evergreen.ProviderNameMock,
				CreationTime: time.Now().Add(-time.Minute * 10),
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			assert.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IncludesHostsThatExceedProvisioningDeadline": func(t *testing.T) {
			h := &Host{
				Id:           "id",
				StartedBy:    evergreen.User,
				CreationTime: time.Now().Add(-time.Hour),
				Provisioned:  false,
				Status:       evergreen.HostStarting,
				Provider:     evergreen.ProviderNameMock,
			}
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
		"IgnoresUserDataHostsThatAreNotInRunningStateButAreRunningTasks": func(t *testing.T) {
			h := &Host{
				Id:          "id",
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
			require.NoError(t, h.Insert(ctx))

			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			assert.Empty(t, toTerminate)
		},
		"IncludesUserDataHostsThatHaveRunTasksBeforeButHaveNotCommunicatedRecently": func(t *testing.T) {
			h := &Host{
				Id:          "id",
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
			require.NoError(t, h.Insert(ctx))
			toTerminate, err := FindHostsToTerminate(ctx)
			require.NoError(t, err)
			require.Len(t, toTerminate, 1)
			assert.Equal(t, h.Id, toTerminate[0].Id)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			tCase(t)
		})
	}
}

func TestGetPaginatedRunningHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.DropCollections(Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"ExcludesTerminatedHosts": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 2)
			require.NotNil(t, hosts[0])
			require.NotEqual(t, hosts[0].Status, evergreen.HostTerminated)
			require.NotNil(t, hosts[1])
			require.NotEqual(t, hosts[1].Status, evergreen.HostTerminated)
		},
		"FilterByID": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "h2",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h2")
		},
		"FilterByDNSName": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "ec2-host-2.compute-1.amazonaws.com",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h2")
		},
		"FilterByDistroID": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "rhel80",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h3")
		},
		"FilterByCurrentTaskID": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "",
					CurrentTaskID: "task_id",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h3")
		},
		"FilterByStatuses": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{evergreen.HostRunning},
					StartedBy:     "",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h2")
		},
		"FilterByStartedBy": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "evergreen",
					SortBy:        "",
					SortDir:       1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h2")
		},
		"SortBy": func(ctx context.Context, t *testing.T) {
			hosts, _, _, err := GetPaginatedRunningHosts(ctx,
				HostsFilterOptions{
					HostID:        "",
					DistroID:      "",
					CurrentTaskID: "",
					Statuses:      []string{},
					StartedBy:     "",
					SortBy:        IdKey,
					SortDir:       -1,
					Page:          0,
					Limit:         0,
				})
			require.NoError(t, err)
			require.Len(t, hosts, 2)
			require.NotNil(t, hosts[0])
			require.Equal(t, hosts[0].Id, "h3")
			require.NotNil(t, hosts[1])
			require.Equal(t, hosts[1].Id, "h2")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
			defer tcancel()

			require.NoError(t, db.Clear(Collection))

			h1 := &Host{
				Id:   "h1",
				Host: "ec2-host-1.compute-1.amazonaws.com",
				Distro: distro.Distro{
					Id: "ubuntu1604-small",
				},
				StartedBy:      "random-user",
				Status:         evergreen.HostTerminated,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-10 * time.Minute),
				RunningTask:    "",
			}
			require.NoError(t, h1.Insert(ctx))

			h2 := &Host{
				Id:   "h2",
				Host: "ec2-host-2.compute-1.amazonaws.com",
				Distro: distro.Distro{
					Id: "ubuntu1604-small",
				},
				StartedBy:      "evergreen",
				Status:         evergreen.HostRunning,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-10 * time.Minute),
				RunningTask:    "",
			}
			require.NoError(t, h2.Insert(ctx))

			h3 := &Host{
				Id:   "h3",
				Host: "ec2-host-3.compute-1.amazonaws.com",
				Distro: distro.Distro{
					Id: "rhel80-large",
				},
				StartedBy:      "admin",
				Status:         evergreen.HostQuarantined,
				Provider:       evergreen.ProviderNameMock,
				ExpirationTime: time.Now().Add(-10 * time.Minute),
				RunningTask:    "task_id",
			}
			require.NoError(t, h3.Insert(ctx))

			tCase(tctx, t)
		})
	}
}

func TestClearDockerStdinData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	require.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id: "host_id",
		DockerOptions: DockerOptions{
			StdinData: []byte("stdin data"),
		},
	}
	require.NoError(t, h.Insert(ctx))

	require.NoError(t, h.ClearDockerStdinData(ctx))
	assert.Empty(t, h.DockerOptions.StdinData)

	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	require.NotZero(t, dbHost)
	assert.Empty(t, dbHost.DockerOptions.StdinData)
}

func TestGeneratePersistentDNSName(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	const domain = "example.com"
	const usernameWithSpecialChars = "it's-a.me,mario!!!🌲"
	validDNSNameRegexp := regexp.MustCompile("[^a-zA-Z0-9.-]")

	t.Run("ReturnsAlreadySetPersistentDNSName", func(t *testing.T) {
		h := Host{
			Id:                "host_id",
			PersistentDNSName: fmt.Sprintf("hello.%s", domain),
		}
		dnsName, err := h.GeneratePersistentDNSName(ctx, domain)
		require.NoError(t, err)
		assert.Equal(t, h.PersistentDNSName, dnsName)
	})
	t.Run("ReturnsNewPersistentDNSNameDeterministically", func(t *testing.T) {
		h := Host{
			Id:        "host_id",
			StartedBy: usernameWithSpecialChars,
		}
		dnsName, err := h.GeneratePersistentDNSName(ctx, domain)
		require.NoError(t, err)
		assert.NotEmpty(t, dnsName)
		assert.True(t, strings.HasSuffix(dnsName, domain), "DNS name should include domain")
		assert.False(t, validDNSNameRegexp.MatchString(dnsName), "generated DNS name should only contain periods, dashes, and alphanumeric characters")
		assert.Zero(t, h.PersistentDNSName, "generated DNS name should not be set")
		// If working properly, the generated DNS name output should always be
		// the same when given the same inputs (i.e. same host ID and host
		// owner) no matter how many times this test runs.
		assert.Equal(t, fmt.Sprintf("itsa-memario-87e.%s", domain), dnsName, "should produce DNS name deterministically if there's no other host with the same DNS name")
	})
	t.Run("AlwaysReturnsSameStringForSameHostID", func(t *testing.T) {
		h := Host{
			Id:        "host_id",
			StartedBy: usernameWithSpecialChars,
		}
		originalDNSName, err := h.GeneratePersistentDNSName(ctx, domain)
		require.NoError(t, err)
		assert.Zero(t, h.PersistentDNSName, "generated DNS name should not be set")
		assert.False(t, validDNSNameRegexp.MatchString(originalDNSName), "generated DNS name should only contain periods, dashes, and alphanumeric characters")

		for i := 0; i < 10; i++ {
			newDNSName, err := h.GeneratePersistentDNSName(ctx, domain)
			require.NoError(t, err)
			assert.Equal(t, originalDNSName, newDNSName, "should always produce the same generated DNS name")
			assert.Zero(t, h.PersistentDNSName, "generated DNS name should not be set")
		}
	})
	t.Run("ReturnsUniqueStringsForDifferentHostIDs", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))
		defer func() {
			assert.NoError(t, db.ClearCollections(Collection))
		}()
		dnsNames := make(map[string]struct{})
		for i := 0; i < 10; i++ {
			h := Host{
				Id:        utility.RandomString(),
				StartedBy: usernameWithSpecialChars,
			}
			dnsName, err := h.GeneratePersistentDNSName(ctx, domain)
			require.NoError(t, err)
			assert.NotContains(t, dnsNames, dnsName, "generated DNS name should be unique")
			assert.False(t, validDNSNameRegexp.MatchString(dnsName), "generated DNS name should only contain periods, dashes, and alphanumeric characters")

			dnsNames[dnsName] = struct{}{}
			h.PersistentDNSName = dnsName
			assert.NoError(t, h.Insert(ctx))
		}
	})
	t.Run("ReturnsUniquePersistentDNSNameEvenIfThereIsAnIdenticalOneAlreadyAssignedToADifferentHost", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(Collection))
		defer func() {
			assert.NoError(t, db.ClearCollections(Collection))
		}()
		h := Host{
			Id:        "host_id",
			StartedBy: usernameWithSpecialChars,
		}

		// Get the DNS name that the host would produce in the absence of
		// collisions.
		originalDNSName, err := h.GeneratePersistentDNSName(ctx, domain)
		require.NoError(t, err)
		assert.NotEmpty(t, originalDNSName)

		// Another host is using that DNS name, so it should generate a
		// different unique one instead.
		conflictingHost := Host{
			Id:                "other_host_id",
			StartedBy:         usernameWithSpecialChars,
			PersistentDNSName: originalDNSName,
		}
		require.NoError(t, conflictingHost.Insert(ctx))

		nonConflictingDNSName, err := h.GeneratePersistentDNSName(ctx, domain)
		require.NoError(t, err)
		assert.NotEmpty(t, nonConflictingDNSName)
		assert.True(t, strings.HasSuffix(nonConflictingDNSName, domain), "DNS name should include domain")
		assert.NotEqual(t, nonConflictingDNSName, conflictingHost.PersistentDNSName, "generated DNS name should not conflict with existing DNS name")
		assert.NotEqual(t, originalDNSName, nonConflictingDNSName, "new DNS name should not be the same as the original one due to conflicting host")
	})
}

func TestSetPersistentDNSInfo(t *testing.T) {
	const dnsName = "hello.example.com"
	const ipv4Addr = "127.0.0.1"
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"SetsPersistentDNSInfo": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetPersistentDNSInfo(ctx, dnsName, ipv4Addr))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, dnsName, dbHost.PersistentDNSName)
			assert.Equal(t, ipv4Addr, dbHost.PublicIPv4)
		},
		"OverwritesExistingPersistentDNSInfo": func(ctx context.Context, t *testing.T, h *Host) {
			h.PersistentDNSName = "bye.example.com"
			h.PublicIPv4 = "0.0.0.0"
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.SetPersistentDNSInfo(ctx, dnsName, ipv4Addr))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Equal(t, dnsName, dbHost.PersistentDNSName)
			assert.Equal(t, ipv4Addr, dbHost.PublicIPv4)
		},
		"FailsWithEmptyDNSName": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, h.SetPersistentDNSInfo(ctx, "", ipv4Addr))
		},
		"FailsWithEmptyIPv4Address": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, h.SetPersistentDNSInfo(ctx, dnsName, ""))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))
			h := &Host{
				Id: "host_id",
			}

			tCase(ctx, t, h)
		})
	}
}

func TestUnsetPersistentDNSInfo(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"Succeeds": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.UnsetPersistentDNSInfo(ctx))
			assert.Zero(t, h.PersistentDNSName)
			assert.Zero(t, h.PublicIPv4)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.PersistentDNSName)
			assert.Zero(t, dbHost.PublicIPv4)
		},
		"NoopsForNonexistentPersistentDNSInfo": func(ctx context.Context, t *testing.T, h *Host) {
			h.PersistentDNSName = ""
			h.PublicIPv4 = ""
			require.NoError(t, h.Insert(ctx))
			assert.NoError(t, h.UnsetPersistentDNSInfo(ctx))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.Zero(t, dbHost.PersistentDNSName)
			assert.Zero(t, dbHost.PublicIPv4)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))
			h := &Host{
				Id:                "host_id",
				PersistentDNSName: "hello.example.com",
				PublicIPv4:        "0.0.0.0",
			}

			tCase(ctx, t, h)
		})
	}
}

func TestNewSleepScheduleInfo(t *testing.T) {
	t.Run("SucceedsWithValidOptions", func(t *testing.T) {
		info, err := NewSleepScheduleInfo(SleepScheduleOptions{
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
			TimeZone:         "Asia/Macau",
		})
		assert.NoError(t, err)
		require.NotZero(t, info)
		assert.Equal(t, []time.Weekday{time.Sunday}, info.WholeWeekdaysOff)
		assert.Zero(t, info.DailyStartTime)
		assert.Zero(t, info.DailyStopTime)
		assert.Equal(t, "Asia/Macau", info.TimeZone)
		assert.NotZero(t, info.NextStartTime)
		assert.NotZero(t, info.NextStopTime)
	})
	t.Run("SucceedsWithDefaultOptions", func(t *testing.T) {
		var defaultOpts SleepScheduleOptions
		defaultOpts.SetDefaultSchedule()
		defaultOpts.SetDefaultTimeZone("America/New_York")
		info, err := NewSleepScheduleInfo(defaultOpts)
		assert.NoError(t, err)
		require.NotZero(t, info)
		assert.ElementsMatch(t, defaultOpts.WholeWeekdaysOff, info.WholeWeekdaysOff)
		assert.Equal(t, defaultOpts.DailyStartTime, info.DailyStartTime)
		assert.Equal(t, defaultOpts.DailyStopTime, info.DailyStopTime)
		assert.Equal(t, defaultOpts.TimeZone, info.TimeZone)
		assert.NotZero(t, info.NextStartTime)
		assert.NotZero(t, info.NextStopTime)
	})
	t.Run("FailsWithZeroOptions", func(t *testing.T) {
		info, err := NewSleepScheduleInfo(SleepScheduleOptions{})
		assert.Error(t, err)
		assert.Zero(t, info)
	})
	t.Run("FailsWithoutTimeZone", func(t *testing.T) {
		var defaultOpts SleepScheduleOptions
		defaultOpts.SetDefaultSchedule()
		info, err := NewSleepScheduleInfo(defaultOpts)
		assert.Error(t, err)
		assert.Zero(t, info)
	})
}

func TestSleepScheduleInfoValidate(t *testing.T) {
	t.Run("FailsWithZeroOptions", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{}).Validate())
	})
	t.Run("SucceedsWithPermanentExemption", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{PermanentlyExempt: true}).Validate())
	})
	t.Run("SucceedsWithOneWholeDayOff", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("SucceedsWithMultipleWholeDayOff", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday, time.Wednesday, time.Friday, time.Saturday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("SucceedsWithDailyScheduleMeetingMinimumHoursPerWeek", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{
			DailyStartTime: "04:00",
			DailyStopTime:  "00:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("SucceedsWithCombinationOfDailyScheduleAndWholeDaysOff", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{
			DailyStartTime:   "05:00",
			DailyStopTime:    "06:00",
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("SucceedsWithDailyOvernightScheduleMeetingMinimumHoursPerWeek", func(t *testing.T) {
		assert.NoError(t, (&SleepScheduleInfo{
			DailyStartTime: "05:00",
			DailyStopTime:  "20:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithNonexistentTimezone", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
			TimeZone:         "foobar",
		}).Validate())
	})
	t.Run("FailsWithoutTimeZone", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
		}).Validate())
	})
	t.Run("FailsWithDailyScheduleUnderMinimumHoursPerWeek", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "01:00",
			DailyStopTime:  "00:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithDailyScheduleUnderOneHourPerDay", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime:   "00:10",
			DailyStopTime:    "00:00",
			WholeWeekdaysOff: []time.Weekday{time.Sunday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("SucceedsWithDailyOvernightScheduleUnderMinimumHoursPerWeek", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "01:00",
			DailyStopTime:  "23:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithOnlyDailyStopTimeAndNoDailyStartTime", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStopTime: "00:00",
			TimeZone:      "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithOnlyDailyStartTimeAndNoDailyStopTime", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "00:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithInvalidDailyStartTime", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "05:00:00",
			DailyStopTime:  "20:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithInvalidDailyStopTime", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "00:00",
			DailyStopTime:  "20:00:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithSameDailyStopAndStartTimes", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			DailyStartTime: "00:00",
			DailyStopTime:  "00:00",
			TimeZone:       "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithDuplicateWholeWeekdaysOff", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday, time.Sunday, time.Monday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithInvalidWeekdays", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{12345},
			TimeZone:         "America/New_York",
		}).Validate())
	})
	t.Run("FailsWithAllWeekdaysOff", func(t *testing.T) {
		assert.Error(t, (&SleepScheduleInfo{
			WholeWeekdaysOff: []time.Weekday{time.Sunday, time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday},
			TimeZone:         "America/New_York",
		}).Validate())
	})
}

func TestUpdateSleepSchedule(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	userTZ, err := time.LoadLocation("America/New_York")
	require.NoError(t, err)

	checkRecurringSleepScheduleMatches := func(t *testing.T, expected SleepScheduleInfo, actual SleepScheduleInfo) {
		assert.Equal(t, expected.WholeWeekdaysOff, actual.WholeWeekdaysOff)
		assert.Equal(t, expected.DailyStartTime, actual.DailyStartTime)
		assert.Equal(t, expected.DailyStopTime, actual.DailyStopTime)
		assert.Equal(t, expected.TimeZone, actual.TimeZone)
	}
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"UpdatesSleepScheduleAndNextScheduledTimes": func(ctx context.Context, t *testing.T, h *Host) {
			now := utility.BSONTime(time.Now())
			require.NoError(t, h.Insert(ctx))
			s := SleepScheduleInfo{
				DailyStartTime: "18:00",
				DailyStopTime:  "06:00",
				TimeZone:       userTZ.String(),
			}

			require.NoError(t, h.UpdateSleepSchedule(ctx, s, now))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkRecurringSleepScheduleMatches(t, s, dbHost.SleepSchedule)

			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be in the future")
			assert.Equal(t, 18, dbHost.SleepSchedule.NextStartTime.In(userTZ).Hour(), "next start time should be at 18:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStartTime.In(userTZ).Minute(), "next start time should be at 18:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStartTime.In(userTZ).Second(), "next start time should be at 18:00 local time")

			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be in the future")
			assert.Equal(t, 6, dbHost.SleepSchedule.NextStopTime.In(userTZ).Hour(), "next stop time should be at 06:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStopTime.In(userTZ).Minute(), "next stop time should be at 06:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStopTime.In(userTZ).Second(), "next stop time should be at 06:00 local time")
			assert.False(t, dbHost.SleepSchedule.PermanentlyExempt)
			assert.True(t, utility.IsZeroTime(dbHost.SleepSchedule.TemporarilyExemptUntil))
			assert.False(t, dbHost.SleepSchedule.ShouldKeepOff)
		},
		"OverwritesExistingSleepScheduleAndNextScheduledTimes": func(ctx context.Context, t *testing.T, h *Host) {
			now := utility.BSONTime(time.Now())
			h.SleepSchedule = SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				NextStartTime:    now.Add(-time.Minute),
				NextStopTime:     now.Add(-time.Minute),
			}
			require.NoError(t, h.Insert(ctx))

			s := SleepScheduleInfo{
				DailyStartTime: "18:00",
				DailyStopTime:  "06:00",
				TimeZone:       userTZ.String(),
			}

			require.NoError(t, h.UpdateSleepSchedule(ctx, s, now))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkRecurringSleepScheduleMatches(t, s, dbHost.SleepSchedule)

			assert.True(t, dbHost.SleepSchedule.NextStartTime.After(now), "next start time should be in the future")
			assert.Equal(t, 18, dbHost.SleepSchedule.NextStartTime.In(userTZ).Hour(), "next start time should be at 18:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStartTime.In(userTZ).Minute(), "next start time should be at 18:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStartTime.In(userTZ).Second(), "next start time should be at 18:00 local time")

			assert.True(t, dbHost.SleepSchedule.NextStopTime.After(now), "next stop time should be in the future")
			assert.Equal(t, 6, dbHost.SleepSchedule.NextStopTime.In(userTZ).Hour(), "next stop time should be at 06:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStopTime.In(userTZ).Minute(), "next stop time should be at 06:00 local time")
			assert.Equal(t, 0, dbHost.SleepSchedule.NextStopTime.In(userTZ).Second(), "next stop time should be at 06:00 local time")

			assert.False(t, dbHost.SleepSchedule.PermanentlyExempt)
			assert.Zero(t, dbHost.SleepSchedule.TemporarilyExemptUntil)
			assert.False(t, dbHost.SleepSchedule.ShouldKeepOff)
		},
		"OverwritesExistingSleepScheduleAndNextScheduledTimesForTemporaryExemption": func(ctx context.Context, t *testing.T, h *Host) {
			now := utility.BSONTime(time.Now())
			temporarilyExemptUntil := utility.BSONTime(now.Add(utility.Day))
			h.SleepSchedule = SleepScheduleInfo{
				WholeWeekdaysOff:       []time.Weekday{time.Sunday},
				NextStartTime:          now.Add(-time.Minute),
				NextStopTime:           now.Add(-time.Minute),
				TemporarilyExemptUntil: temporarilyExemptUntil,
			}
			require.NoError(t, h.Insert(ctx))

			s := SleepScheduleInfo{
				DailyStartTime:         "18:00",
				DailyStopTime:          "06:00",
				TimeZone:               userTZ.String(),
				TemporarilyExemptUntil: temporarilyExemptUntil,
			}

			require.NoError(t, h.UpdateSleepSchedule(ctx, s, now))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkRecurringSleepScheduleMatches(t, s, dbHost.SleepSchedule)

			assert.Zero(t, dbHost.SleepSchedule.NextStartTime, "next start time should be unset during temporary exemption")
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime, "next stop time should be unset during temporary exemption")

			assert.False(t, dbHost.SleepSchedule.PermanentlyExempt)
			assert.True(t, temporarilyExemptUntil.Equal(dbHost.SleepSchedule.TemporarilyExemptUntil))
			assert.False(t, dbHost.SleepSchedule.ShouldKeepOff)
		},
		"NextStartAndStopTimesIsBasedOnCurrentTime": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			const easternTZ = "America/New_York"
			easternTZLoc, err := time.LoadLocation(easternTZ)
			require.NoError(t, err)

			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 15:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 15:00:00", easternTZLoc)
			require.NoError(t, err)
			now = utility.BSONTime(now.UTC())

			s := SleepScheduleInfo{
				DailyStartTime: "10:00",
				DailyStopTime:  "18:00",
				TimeZone:       userTZ.String(),
			}

			require.NoError(t, h.UpdateSleepSchedule(ctx, s, now))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkRecurringSleepScheduleMatches(t, s, dbHost.SleepSchedule)

			expectedNextStartTime, err := time.ParseInLocation(time.DateTime, "2024-02-22 10:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStartTime, dbHost.SleepSchedule.NextStartTime, 0, "next start time should be at 10:00 local time on the next day")

			expectedNextStopTime, err := time.ParseInLocation(time.DateTime, "2024-02-21 18:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStopTime, dbHost.SleepSchedule.NextStopTime, 0, "next stop time should be at 18:00 local time on the same day")
		},
		"FailsWithZeroSleepSchedule": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, h.UpdateSleepSchedule(ctx, SleepScheduleInfo{}, time.Now()))
		},
		"FailsWithInvalidSleepSchedule": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			assert.Error(t, h.UpdateSleepSchedule(ctx, SleepScheduleInfo{
				DailyStartTime: "10:00",
			}, time.Now()))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))

			tCase(ctx, t, &Host{
				Id:           "host_id",
				NoExpiration: true,
				StartedBy:    "me",
				Status:       evergreen.HostRunning,
			})
		})
	}
}

func TestSetTemporaryExemption(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"SetsTemporaryExemptionAndClearsNextScheduledTimes": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			now := utility.BSONTime(time.Now())
			exemptUntil := utility.BSONTime(now.Add(2 * utility.Day))
			require.NoError(t, h.SetTemporaryExemption(ctx, exemptUntil))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.SleepSchedule.TemporarilyExemptUntil.Equal(exemptUntil), "should set temporary exemption time to '%s'", exemptUntil)
			assert.Zero(t, dbHost.SleepSchedule.NextStartTime, "should clear next start time")
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime, "should clear next stop time")
		},
		"FailsWithVeryLongTemporaryExemption": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			now := utility.BSONTime(time.Now())
			exemptUntil := utility.BSONTime(now.Add(1000 * utility.Day))
			assert.Error(t, h.SetTemporaryExemption(ctx, exemptUntil))
		},
		"OverwritesExistingTemporaryExemption": func(ctx context.Context, t *testing.T, h *Host) {
			now := utility.BSONTime(time.Now())
			h.SleepSchedule.TemporarilyExemptUntil = utility.BSONTime(now.Add(time.Hour))
			require.NoError(t, h.Insert(ctx))

			exemptUntil := utility.BSONTime(now.Add(2 * utility.Day))
			require.NoError(t, h.SetTemporaryExemption(ctx, exemptUntil))

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			assert.True(t, dbHost.SleepSchedule.TemporarilyExemptUntil.Equal(exemptUntil), "should update temporary exemption time to '%s'", exemptUntil)
			assert.Zero(t, dbHost.SleepSchedule.NextStartTime, "should clear next start time")
			assert.Zero(t, dbHost.SleepSchedule.NextStopTime, "should clear next stop time")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			require.NoError(t, db.ClearCollections(Collection))
			h := &Host{
				Id:           "host_id",
				NoExpiration: true,
				StartedBy:    "me",
				SleepSchedule: SleepScheduleInfo{
					TimeZone:       time.Local.String(),
					DailyStartTime: "10:00",
					DailyStopTime:  "18:00",
				},
			}
			tCase(ctx, t, h)
		})
	}
}

func TestGetNextScheduledStopTime(t *testing.T) {
	const easternTZ = "America/New_York"
	easternTZLoc, err := time.LoadLocation(easternTZ)
	require.NoError(t, err)

	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsNextStopTimeForWholeDayOff": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				TimeZone:         easternTZ,
			}
			now := time.Now()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			assert.True(t, nextStop.After(now), "next stop time should be in the future")
			assert.True(t, nextStop.Compare(now.AddDate(0, 0, 7)) <= 0, "next stop time should within the next week")
			assert.Equal(t, time.Sunday, nextStop.Weekday(), "next stop time should be on a Sunday")
			assert.Zero(t, nextStop.Hour(), "next stop time should be at midnight")
			assert.Zero(t, nextStop.Minute(), "next stop time should be at midnight")
			assert.Zero(t, nextStop.Second(), "next stop time should be at midnight")
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeBasedOnCurrentTimestamp": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday, time.Tuesday, time.Thursday},
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 15:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 15:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Thursday February 22, 2024 at 00:00 EST
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-22 00:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")

			// The next stop time should be equivalent to:
			// Thursday February 22, 2024 at 05:00 UTC
			expectedNextStopUTC, err := time.ParseInLocation(time.DateTime, "2024-02-22 05:00:00", time.UTC)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStopUTC, nextStop, 0)
		},
		"ReturnsNextStopTimeCorrectlyWithTimeZoneDifferenceBetweenUserTimeAndEvergreenTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Wednesday},
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 00:00 UTC
			// Note that the user time zone is in EST, which is 5 hours behind
			// UTC, so in the user's time zone, it's Wednesday February 21, 2024
			// at 19:00 EST.
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 00:00:00", time.UTC)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Wednesday February 21, 2024 at 00:00 EST
			// This is because the current time is still Tuesday
			// 19:00 in EST.
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-21 00:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeForDailySchedule": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "17:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 01:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 01:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Wednesday February 21, 2024 at 17:00 EST
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-21 17:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeForDailyScheduleAfterStopTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "17:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 17:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 17:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Thursday February 22, 2024 at 17:00 EST
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-22 17:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeAsWholeWeekdayOffAfterDailyStop": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				DailyStartTime:   "06:00",
				DailyStopTime:    "01:00",
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Friday February 23, 2024 at 19:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-23 19:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Saturday February 24, 2024 at 00:00 EST
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-24 00:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeAsDailyStopTimeForOvernightDailyScheduleLeadingIntoWholeWeekdaysOff": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				DailyStartTime:   "06:00",
				DailyStopTime:    "22:00",
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Friday February 23, 2024 at 21:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-23 21:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Friday February 23, 2024 at 22:00 EST
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-23 22:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeAfterNextStartTime": func(t *testing.T) {
			// Simulate the next start time, which is:
			// Monday February 26, 2024 at 06:00 EST
			nextStart, err := time.ParseInLocation(time.DateTime, "2024-02-26 06:00:00", easternTZLoc)
			require.NoError(t, err)
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				DailyStartTime:   "06:00",
				DailyStopTime:    "22:00",
				NextStartTime:    nextStart,
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Friday February 23, 2024 at 22:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-23 22:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Monday February 26, 2024 at 22:00
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-02-26 22:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsNextStopTimeWithAccountingForDaylightSavingsTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "22:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Saturday March 9, 2024 at 22:00 EST (AKA Sunday March 10, 2024 at
			// 03:00 UTC)
			now, err := time.ParseInLocation(time.DateTime, "2024-03-09 22:00:00", easternTZLoc)
			require.NoError(t, err)
			_, tzOffset := now.Zone()
			expectedOffsetSecs := int((-5 * time.Hour).Seconds())
			assert.Equal(t, expectedOffsetSecs, tzOffset, "current time should be EST")
			now = now.UTC()

			nextStop, err := s.GetNextScheduledStopTime(now)
			assert.NoError(t, err)

			// The next stop time should be:
			// Sunday March 10, 2024 at 22:00 EDT
			// Note that daylight savings time begins on March 10 at 2 AM,
			// meaning that the next stop time is in EDT rather than EST.
			expectedNextStop, err := time.ParseInLocation(time.DateTime, "2024-03-10 22:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStop, nextStop, 0)

			_, tzOffset = nextStop.Zone()
			expectedOffsetSecs = int((-4 * time.Hour).Seconds())
			assert.Equal(t, expectedOffsetSecs, tzOffset, "next stop time should be in EDT rather than EST")
			assert.Equal(t, s.TimeZone, nextStop.Location().String(), "next stop time should be specified in the user's time zone")
		},
		"ReturnsZeroTimeForPermanentlyExemptHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff:  []time.Weekday{time.Sunday},
				PermanentlyExempt: true,
			}
			nextStop, err := s.GetNextScheduledStopTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStop)
		},
		"ReturnsZeroTimeForIndefinitelyOffHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				ShouldKeepOff:    true,
			}
			nextStop, err := s.GetNextScheduledStopTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStop)
		},
		"ReturnsZeroTimeForTemporarilyExemptHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff:       []time.Weekday{time.Sunday},
				TemporarilyExemptUntil: time.Now().Add(utility.Day),
			}
			nextStop, err := s.GetNextScheduledStopTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStop)
		},
		"ReturnsErrorForZeroSleepSchedule": func(t *testing.T) {
			s := SleepScheduleInfo{}
			_, err := s.GetNextScheduledStopTime(time.Now())
			assert.Error(t, err)
		},
		"ReturnsErrorForInvalidTimeZone": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				TimeZone:         "foobar",
			}
			_, err := s.GetNextScheduledStopTime(time.Now())
			assert.Error(t, err)
		},
	} {
		t.Run(tName, tCase)
	}
}

func TestGetNextScheduledStartTime(t *testing.T) {
	const easternTZ = "America/New_York"
	easternTZLoc, err := time.LoadLocation(easternTZ)
	require.NoError(t, err)

	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsNextStartTimeForWholeDayOff": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				TimeZone:         easternTZ,
			}
			now := time.Now()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			assert.True(t, nextStart.After(now), "next start time should be in the future")
			assert.True(t, nextStart.Compare(now.AddDate(0, 0, 7)) <= 0, "next start time should within the next week")
			assert.Equal(t, time.Monday, nextStart.Weekday(), "next start time should be on Monday")
			assert.Zero(t, nextStart.Hour(), "next start time should be at midnight")
			assert.Zero(t, nextStart.Minute(), "next start time should be at midnight")
			assert.Zero(t, nextStart.Second(), "next start time should be at midnight")
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeBasedOnCurrentTimestamp": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday, time.Tuesday, time.Thursday},
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 15:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 15:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Friday February 23, 2024 at 00:00 EST
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-23 00:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")

			// The next start time should be equivalent to:
			// Friday February 23, 2024 at 05:00 UTC
			expectedNextStartUTC, err := time.ParseInLocation(time.DateTime, "2024-02-23 05:00:00", time.UTC)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStartUTC, nextStart, 0)
		},
		"ReturnsNextStartTimeCorrectlyWithTimeZoneDifferenceBetweenUserTimeAndEvergreenTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Wednesday},
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Thursday February 22, 2024 at 00:00 UTC
			// Note that the user time zone is in EST, which is 5 hours behind
			// UTC, so in the user's time zone, it's Wednesday February 21, 2024
			// at 19:00 EST.
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 00:00:00", time.UTC)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Thursday February 21, 2024 at 00:00 EST
			// This is because the current time is still Wednesday
			// 19:00 in EST.
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-22 00:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeForDailySchedule": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "17:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 01:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 01:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Wednesday February 22, 2024 at 06:00 EST
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-21 06:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeForDailyScheduleAfterStartTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "17:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Wednesday February 21, 2024 at 10:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-21 06:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Thursday February 22, 2024 at 06:00 EST
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-22 06:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeAsMidnightAfterWholeWeekdayOffAfterDailyStart": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				DailyStartTime:   "06:00",
				DailyStopTime:    "22:00",
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Friday February 23, 2024 at 07:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-23 07:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Monday February 26, 2024 at 07:00 EST
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-26 06:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeAsDailyStartTimeForWholeWeekdaysOffLeadingIntoOvernightDailySchedule": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Saturday, time.Sunday},
				DailyStartTime:   "06:00",
				DailyStopTime:    "22:00",
				TimeZone:         easternTZ,
			}
			// Simulate the current time, which is:
			// Friday February 23, 2024 at 21:00 EST
			now, err := time.ParseInLocation(time.DateTime, "2024-02-23 21:00:00", easternTZLoc)
			require.NoError(t, err)
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Monday February 26, 2024 at 06:00 EST
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-02-26 06:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsNextStartTimeWithAccountingForDaylightSavingsTime": func(t *testing.T) {
			s := SleepScheduleInfo{
				DailyStartTime: "06:00",
				DailyStopTime:  "22:00",
				TimeZone:       easternTZ,
			}
			// Simulate the current time, which is:
			// Saturday March 9, 2024 at 22:00 EST (AKA Sunday March 10, 2024 at
			// 03:00 UTC)
			now, err := time.ParseInLocation(time.DateTime, "2024-03-09 22:00:00", easternTZLoc)
			require.NoError(t, err)
			_, tzOffset := now.Zone()
			expectedOffsetSecs := int((-5 * time.Hour).Seconds())
			assert.Equal(t, expectedOffsetSecs, tzOffset, "current time should be EST")
			now = now.UTC()

			nextStart, err := s.GetNextScheduledStartTime(now)
			assert.NoError(t, err)

			// The next start time should be:
			// Sunday March 10, 2024 at 02:30 EDT
			// Note that daylight savings time begins on March 10 at 2 AM,
			// meaning that the next start time is in EDT rather than EST.
			expectedNextStart, err := time.ParseInLocation(time.DateTime, "2024-03-10 06:00:00", easternTZLoc)
			require.NoError(t, err)
			assert.WithinDuration(t, expectedNextStart, nextStart, 0)

			_, tzOffset = nextStart.Zone()
			expectedOffsetSecs = int((-4 * time.Hour).Seconds())
			assert.Equal(t, expectedOffsetSecs, tzOffset, "next start time should be in EDT rather than EST")
			assert.Equal(t, s.TimeZone, nextStart.Location().String(), "next start time should be specified in the user's time zone")
		},
		"ReturnsZeroTimeForPermanentlyExemptHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff:  []time.Weekday{time.Sunday},
				PermanentlyExempt: true,
			}
			nextStop, err := s.GetNextScheduledStartTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStop)
		},
		"ReturnsZeroTimeForIndefinitelyOffHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				ShouldKeepOff:    true,
			}
			nextStop, err := s.GetNextScheduledStartTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStop)
		},
		"ReturnsZeroTimeForTemporarilyExemptHost": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff:       []time.Weekday{time.Sunday},
				TemporarilyExemptUntil: time.Now().Add(utility.Day),
			}
			nextStart, err := s.GetNextScheduledStartTime(time.Now())
			assert.NoError(t, err)
			assert.Zero(t, nextStart)
		},
		"ReturnsErrorForZeroSleepSchedule": func(t *testing.T) {
			s := SleepScheduleInfo{}
			_, err := s.GetNextScheduledStartTime(time.Now())
			assert.Error(t, err)
		},
		"ReturnsErrorForInvalidTimeZone": func(t *testing.T) {
			s := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Sunday},
				TimeZone:         "foobar",
			}
			_, err := s.GetNextScheduledStartTime(time.Now())
			assert.Error(t, err)
		},
	} {
		t.Run(tName, tCase)
	}
}

func TestMarkShouldNotExpire(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	const expireOn = "expire_on"
	// checkUnexpirable verifies that the expected unexpirable host fields are
	// set and that it has valid sleep schedule settings.
	checkUnexpirable := func(t *testing.T, h *Host) {
		assert.True(t, h.NoExpiration)
		assert.True(t, h.ExpirationTime.After(time.Now()))
		assert.ElementsMatch(t, []Tag{
			{
				Key:   evergreen.TagExpireOn,
				Value: expireOn,
			},
		}, h.InstanceTags)

		assert.NotZero(t, h.SleepSchedule)
		assert.NoError(t, h.SleepSchedule.Validate(), "should set a default sleep schedule")
	}

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, h *Host){
		"SetsUnexpirableFields": func(ctx context.Context, t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
			assert.NotZero(t, dbHost.SleepSchedule)
			assert.NoError(t, dbHost.SleepSchedule.Validate(), "should set a default sleep schedule")
		},
		"BumpsExpirationForAlreadyUnexpirableHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = true
			h.ExpirationTime = time.Now().Add(-time.Hour)
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
		},
		"DoesNotOverrideValidSleepSchedule": func(ctx context.Context, t *testing.T, h *Host) {
			initialSchedule := SleepScheduleInfo{
				WholeWeekdaysOff: []time.Weekday{time.Friday, time.Saturday, time.Sunday},
				TimeZone:         time.Local.String(),
			}
			assert.NoError(t, initialSchedule.Validate(), "initial sleep schedule should be valid")
			h.SleepSchedule = initialSchedule
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
			assert.Equal(t, initialSchedule, dbHost.SleepSchedule, "should not modify existing valid sleep schedule")
		},
		"DoesNotOverridePermanentExemptionForHost": func(ctx context.Context, t *testing.T, h *Host) {
			h.SleepSchedule.PermanentlyExempt = true
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)
			assert.True(t, h.SleepSchedule.PermanentlyExempt, "should remain permanently exempt from sleep schedule")

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
			assert.True(t, dbHost.SleepSchedule.PermanentlyExempt, "should remain permanently exempt from sleep schedule")
		},
		"SetsDefaultSleepScheduleForUnexpirableHostMissingOne": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = true
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
		},
		"SetsDefaultSleepScheduleForHostWithInvalidSchedule": func(ctx context.Context, t *testing.T, h *Host) {
			h.NoExpiration = true
			h.SleepSchedule = SleepScheduleInfo{
				TimeZone: "foobar",
			}
			require.NoError(t, h.Insert(ctx))

			require.NoError(t, h.MarkShouldNotExpire(ctx, expireOn, time.Local.String()))
			checkUnexpirable(t, h)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			require.NotZero(t, dbHost)
			checkUnexpirable(t, h)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			require.NoError(t, db.ClearCollections(Collection))

			tCase(ctx, t, &Host{Id: "host_id"})
		})
	}
}
