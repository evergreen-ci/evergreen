package host

import (
	"context"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	adb "github.com/mongodb/anser/db"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// IsActive is a query that returns all Evergreen hosts that are working or
// capable of being assigned work to do.
var IsActive = db.Query(
	bson.M{
		StartedByKey: evergreen.User,
		StatusKey: bson.M{
			"$nin": []string{
				evergreen.HostTerminated, evergreen.HostDecommissioned,
			},
		},
	},
)

func hostIdInSlice(hosts []Host, id string) bool {
	for _, host := range hosts {
		if host.Id == id {
			return true
		}
	}
	return false
}

func TestGenericHostFinding(t *testing.T) {

	Convey("When finding hosts", t, func() {
		require.NoError(t, db.Clear(Collection), "Error clearing"+
			" '%v' collection", Collection)

		Convey("when finding one host", func() {
			Convey("the matching host should be returned", func() {
				matchingHost := &Host{
					Id: "matches",
				}
				So(matchingHost.Insert(), ShouldBeNil)

				nonMatchingHost := &Host{
					Id: "nonMatches",
				}
				So(nonMatchingHost.Insert(), ShouldBeNil)

				found, err := FindOne(ById(matchingHost.Id))
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
				So(matchingHostOne.Insert(), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Distro: distro.Distro{Id: "d1"},
				}
				So(matchingHostTwo.Insert(), ShouldBeNil)

				nonMatchingHost := &Host{
					Id:     "nonMatches",
					Distro: distro.Distro{Id: "d2"},
				}
				So(nonMatchingHost.Insert(), ShouldBeNil)

				found, err := Find(db.Query(bson.M{dId: "d1"}))
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 2)
				So(hostIdInSlice(found, matchingHostOne.Id), ShouldBeTrue)
				So(hostIdInSlice(found, matchingHostTwo.Id), ShouldBeTrue)

			})

			Convey("when querying two hosts for running tasks", func() {
				matchingHost := &Host{Id: "task", Status: evergreen.HostRunning, RunningTask: "t1"}
				So(matchingHost.Insert(), ShouldBeNil)
				nonMatchingHost := &Host{Id: "nope", Status: evergreen.HostRunning}
				So(nonMatchingHost.Insert(), ShouldBeNil)
				Convey("the host with the running task should be returned", func() {
					found, err := Find(IsRunningTask)
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
				So(matchingHostOne.Insert(), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Host:   "hostTwo",
					Distro: distro.Distro{Id: "d1"},
					Tag:    "1",
				}
				So(matchingHostTwo.Insert(), ShouldBeNil)

				matchingHostThree := &Host{
					Id:     "stillMatches",
					Host:   "hostThree",
					Distro: distro.Distro{Id: "d1"},
					Tag:    "3",
				}
				So(matchingHostThree.Insert(), ShouldBeNil)

				// find the hosts, removing the host field from the projection,
				// sorting by tag, skipping one, and limiting to one

				found, err := Find(db.Query(bson.M{dId: "d1"}).
					WithoutFields(DNSKey).
					Sort([]string{TagKey}).
					Skip(1).Limit(1))
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 1)
				So(found[0].Id, ShouldEqual, matchingHostOne.Id)
				So(found[0].Host, ShouldEqual, "") // filtered out in projection
			})
		})
	})
}

func TestFindingHostsWithRunningTasks(t *testing.T) {
	Convey("With a host with no running task that is not terminated", t, func() {
		require.NoError(t, db.Clear(Collection), "Error clearing"+
			" '%v' collection", Collection)
		h := Host{
			Id:     "sample_host",
			Status: evergreen.HostRunning,
		}
		So(h.Insert(), ShouldBeNil)
		found, err := Find(IsRunningTask)
		So(err, ShouldBeNil)
		So(len(found), ShouldEqual, 0)
		Convey("with a host that is terminated with no running task", func() {
			require.NoError(t, db.Clear(Collection), "Error clearing"+
				" '%v' collection", Collection)
			h1 := Host{
				Id:     "another",
				Status: evergreen.HostTerminated,
			}
			So(h1.Insert(), ShouldBeNil)
			found, err = Find(IsRunningTask)
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 0)
		})
	})

}

func TestMonitorHosts(t *testing.T) {
	Convey("With a host with no reachability check", t, func() {
		require.NoError(t, db.Clear(Collection), "Error clearing"+
			" '%v' collection", Collection)
		now := time.Now()
		h := Host{
			Id:        "sample_host",
			Status:    evergreen.HostRunning,
			StartedBy: evergreen.User,
		}
		So(h.Insert(), ShouldBeNil)
		found, err := Find(ByNotMonitoredSince(now))
		So(err, ShouldBeNil)
		So(len(found), ShouldEqual, 1)
		Convey("a host that has a running task and no reachability check should not return", func() {
			require.NoError(t, db.Clear(Collection), "Error clearing"+
				" '%v' collection", Collection)
			anotherHost := Host{
				Id:          "anotherHost",
				Status:      evergreen.HostRunning,
				StartedBy:   evergreen.User,
				RunningTask: "id",
			}
			So(anotherHost.Insert(), ShouldBeNil)
			found, err := Find(ByNotMonitoredSince(now))
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 0)
		})
	})
}

func TestUpdatingHostStatus(t *testing.T) {

	Convey("With a host", t, func() {
		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(), ShouldBeNil)

		Convey("setting the host's status should update both the in-memory"+
			" and database versions of the host", func() {

			So(host.SetStatus(evergreen.HostRunning, evergreen.User, ""), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

		})

		Convey("if the host is terminated, the status update should fail"+
			" with an error", func() {

			So(host.SetStatus(evergreen.HostTerminated, evergreen.User, ""), ShouldBeNil)
			So(host.SetStatus(evergreen.HostRunning, evergreen.User, ""), ShouldNotBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)

		})

	})

}

func TestSetHostTerminated(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(), ShouldBeNil)

		Convey("setting the host as terminated should set the status and the"+
			" termination time in both the in-memory and database copies of"+
			" the host", func() {

			So(host.Terminate(evergreen.User, ""), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

		})

	})
}

func TestHostSetDNSName(t *testing.T) {
	var err error

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(), ShouldBeNil)

		Convey("setting the hostname should update both the in-memory and"+
			" database copies of the host", func() {

			So(host.SetDNSName("hostname"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")
			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			// if the host is already updated, no new updates should work
			So(host.SetDNSName("hostname2"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

		})

	})
}

func TestHostSetIPv6Address(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host := &Host{
		Id: "hostOne",
	}
	assert.NoError(host.Insert())

	ipv6Address := "abcd:1234:459c:2d00:cfe4:843b:1d60:8e47"
	ipv6Address2 := "aaaa:1f18:459c:2d00:cfe4:843b:1d60:9999"

	assert.NoError(host.SetIPv6Address(ipv6Address))
	assert.Equal(host.IP, ipv6Address)
	host, err := FindOne(ById(host.Id))
	assert.NoError(err)
	assert.Equal(host.IP, ipv6Address)

	// if the host is already updated, new updates should work
	assert.NoError(host.SetIPv6Address(ipv6Address2))
	assert.Equal(host.IP, ipv6Address2)

	host, err = FindOne(ById(host.Id))
	assert.NoError(err)
	assert.Equal(host.IP, ipv6Address2)
}

func TestMarkAsProvisioned(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		var err error

		host := &Host{
			Id: "hostOne",
		}

		host2 := &Host{
			Id:     "hostTwo",
			Status: evergreen.HostTerminated,
		}

		So(host.Insert(), ShouldBeNil)
		So(host2.Insert(), ShouldBeNil)
		Convey("marking a host that isn't down as provisioned should update the status,"+
			" provisioned, and host name fields in both the in-memory and"+
			" database copies of the host", func() {

			So(host.MarkAsProvisioned(), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			So(host2.MarkAsProvisioned().Error(), ShouldContainSubstring, "not found")
			So(host2.Status, ShouldEqual, evergreen.HostTerminated)
			So(host2.Provisioned, ShouldEqual, false)
		})

	})
}

func TestHostCreateSecret(t *testing.T) {
	Convey("With a host with no secret", t, func() {

		require.NoError(t, db.Clear(Collection),
			"Error clearing '%v' collection", Collection)

		host := &Host{Id: "hostOne"}
		So(host.Insert(), ShouldBeNil)

		Convey("creating a secret", func() {
			So(host.Secret, ShouldEqual, "")
			So(host.CreateSecret(), ShouldBeNil)

			Convey("should update the host in memory", func() {
				So(host.Secret, ShouldNotEqual, "")

				Convey("and in the database", func() {
					dbHost, err := FindOne(ById(host.Id))
					So(err, ShouldBeNil)
					So(dbHost.Secret, ShouldEqual, host.Secret)
				})
			})
		})
	})
}

func TestHostSetAgentStartTime(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()

	h := &Host{
		Id: "id",
	}
	require.NoError(t, h.Insert())

	now := time.Now()
	require.NoError(t, h.SetAgentStartTime())
	assert.True(t, now.Sub(h.AgentStartTime) < time.Second)

	dbHost, err := FindOneId(h.Id)
	require.NoError(t, err)
	assert.True(t, now.Sub(dbHost.AgentStartTime) < time.Second)
}

func TestHostSetExpirationTime(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		initialExpirationTime := time.Now()
		notifications := make(map[string]bool)
		notifications["2h"] = true

		memHost := &Host{
			Id:             "hostOne",
			NoExpiration:   true,
			ExpirationTime: initialExpirationTime,
			Notifications:  notifications,
		}
		So(memHost.Insert(), ShouldBeNil)

		Convey("setting the expiration time for the host should change the "+
			" expiration time for both the in-memory and database"+
			" copies of the host and unset the notifications", func() {

			dbHost, err := FindOne(ById(memHost.Id))

			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.NoExpiration, ShouldBeTrue)
			So(dbHost.NoExpiration, ShouldBeTrue)
			So(memHost.ExpirationTime.Round(time.Second).Equal(
				initialExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(dbHost.ExpirationTime.Round(time.Second).Equal(
				initialExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(memHost.Notifications, ShouldResemble, notifications)
			So(dbHost.Notifications, ShouldResemble, notifications)

			// now update the expiration time
			newExpirationTime := time.Now()
			So(memHost.SetExpirationTime(newExpirationTime), ShouldBeNil)

			dbHost, err = FindOne(ById(memHost.Id))

			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.NoExpiration, ShouldBeFalse)
			So(dbHost.NoExpiration, ShouldBeFalse)
			So(memHost.ExpirationTime.Round(time.Second).Equal(
				newExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(dbHost.ExpirationTime.Round(time.Second).Equal(
				newExpirationTime.Round(time.Second)), ShouldBeTrue)
			So(memHost.Notifications, ShouldResemble, make(map[string]bool))
			So(dbHost.Notifications, ShouldEqual, nil)
		})
	})
}

func TestSetExpirationNotification(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		notifications := make(map[string]bool)
		notifications["2h"] = true

		memHost := &Host{
			Id:            "hostOne",
			Notifications: notifications,
		}
		So(memHost.Insert(), ShouldBeNil)

		Convey("setting the expiration notification for the host should change "+
			" the expiration notification for both the in-memory and database"+
			" copies of the host and unset the notifications", func() {

			dbHost, err := FindOne(ById(memHost.Id))

			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.Notifications, ShouldResemble, notifications)
			So(dbHost.Notifications, ShouldResemble, notifications)

			// now update the expiration notification
			notifications["4h"] = true
			So(memHost.SetExpirationNotification("4h"), ShouldBeNil)
			dbHost, err = FindOne(ById(memHost.Id))
			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.Notifications, ShouldResemble, notifications)
			So(dbHost.Notifications, ShouldResemble, notifications)
		})
	})
}

func TestHostClearRunningAndSetLastTask(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		var err error
		var count int

		host := &Host{
			Id:          "hostOne",
			RunningTask: "taskId",
			StartedBy:   evergreen.User,
			Status:      evergreen.HostRunning,
		}

		So(host.Insert(), ShouldBeNil)

		Convey("host statistics should properly count this host as active"+
			" but not idle", func() {
			count, err = Count(IsActive)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			count, err = Count(IsIdle)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("clearing the running task should clear the running task, pid,"+
			" and task dispatch time fields from both the in-memory and"+
			" database copies of the host", func() {

			So(host.ClearRunningAndSetLastTask(&task.Task{Id: "prevTask"}), ShouldBeNil)
			So(host.RunningTask, ShouldEqual, "")
			So(host.LastTask, ShouldEqual, "prevTask")

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)

			So(host.RunningTask, ShouldEqual, "")
			So(host.LastTask, ShouldEqual, "prevTask")

			Convey("the count of idle hosts should go up", func() {
				count, err := Count(IsIdle)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)

				Convey("but the active host count should remain the same", func() {
					count, err = Count(IsActive)
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1)
				})
			})

		})

	})
}

func TestUpdateHostRunningTask(t *testing.T) {
	Convey("With a host", t, func() {
		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)
		oldTaskId := "oldId"
		newTaskId := "newId"
		h := Host{
			Id:     "test",
			Status: evergreen.HostRunning,
		}
		So(h.Insert(), ShouldBeNil)
		Convey("updating the running task id should set proper fields", func() {
			_, err := h.UpdateRunningTask(&task.Task{Id: newTaskId})
			So(err, ShouldBeNil)
			found, err := FindOne(ById(h.Id))
			So(err, ShouldBeNil)
			So(found.RunningTask, ShouldEqual, newTaskId)
			runningTaskHosts, err := Find(IsRunningTask)
			So(err, ShouldBeNil)
			So(len(runningTaskHosts), ShouldEqual, 1)
		})
		Convey("updating the running task to an empty string should error out", func() {
			_, err := h.UpdateRunningTask(&task.Task{})
			So(err, ShouldNotBeNil)
		})
		Convey("updating the running task when a task is already running should error", func() {
			_, err := h.UpdateRunningTask(&task.Task{Id: oldTaskId})
			So(err, ShouldBeNil)
			_, err = h.UpdateRunningTask(&task.Task{Id: newTaskId})
			So(err, ShouldNotBeNil)
		})
	})
}

func TestUpsert(t *testing.T) {

	Convey("With a host", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id:     "hostOne",
			Host:   "host",
			User:   "user",
			Distro: distro.Distro{Id: "distro"},
			Status: evergreen.HostRunning,
		}

		var err error

		Convey("Performing a host upsert should upsert correctly", func() {
			_, err = host.Upsert()
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

		})

		Convey("Updating some fields of an already inserted host should cause "+
			"those fields to be updated but should leave status unchanged",
			func() {
				_, err := host.Upsert()
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostRunning)

				host, err = FindOne(ById(host.Id))
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostRunning)
				So(host.Host, ShouldEqual, "host")

				err = UpdateOne(
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
				_, err = host.Upsert()
				So(err, ShouldBeNil)

				// host db status should remain unchanged
				host, err = FindOne(ById(host.Id))
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, evergreen.HostDecommissioned)
				So(host.Host, ShouldEqual, "host2")

			})

		Convey("Upserting a host with new ID should set priv_atttempts", func() {
			So(host.Insert(), ShouldBeNil)
			So(host.Remove(), ShouldBeNil)
			host.Id = "s-12345"
			_, err := host.Upsert()
			So(err, ShouldBeNil)

			out := bson.M{}
			So(db.FindOneQ(Collection, db.Query(bson.M{}), &out), ShouldBeNil)
			val, ok := out[ProvisionAttemptsKey]
			So(ok, ShouldBeTrue)
			So(val, ShouldEqual, 0)
		})
	})
}

func TestDecommissionHostsWithDistroId(t *testing.T) {

	Convey("With a multiple hosts of different distros", t, func() {

		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)

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

			require.NoError(t, hostWithDistroA.Insert(), "Error inserting"+
				"host into database")
			require.NoError(t, hostWithDistroB.Insert(), "Error inserting"+
				"host into database")
		}

		Convey("When decommissioning hosts of type distro_a", func() {
			err := DecommissionHostsWithDistroId(distroA)
			So(err, ShouldBeNil)

			Convey("Distro should be marked as decommissioned accordingly", func() {
				hostsTypeA, err := Find(ByDistroId(distroA))
				So(err, ShouldBeNil)

				hostsTypeB, err := Find(ByDistroId(distroB))
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
	Convey("with the a given time for checking and an empty hosts collection", t, func() {
		require.NoError(t, db.Clear(Collection), "Error"+
			" clearing '%v' collection", Collection)
		now := time.Now()
		Convey("with a host that has no last communication time", func() {
			h := Host{
				Id:        "id",
				Status:    evergreen.HostRunning,
				StartedBy: evergreen.User,
			}
			So(h.Insert(), ShouldBeNil)
			hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(time.Now())))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, "id")
			Convey("after unsetting the host's lct", func() {
				err := UpdateOne(bson.M{IdKey: h.Id},
					bson.M{
						"$unset": bson.M{LastCommunicationTimeKey: 0},
					})
				So(err, ShouldBeNil)
				foundHost, err := FindOne(ById(h.Id))
				So(err, ShouldBeNil)
				So(foundHost, ShouldNotBeNil)
				hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(time.Now())))
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
			}
			So(anotherHost.Insert(), ShouldBeNil)
			hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(now)))
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
			}
			So(anotherHost.Insert(), ShouldBeNil)
			hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(now)))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
			Convey("after resetting the LCT", func() {
				So(anotherHost.ResetLastCommunicated(), ShouldBeNil)
				So(anotherHost.LastCommunicationTime, ShouldResemble, time.Unix(0, 0))
				h, err := Find(db.Query(AgentLastCommunicationTimeElapsed(now)))
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
			So(h.Insert(), ShouldBeNil)
			hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(now)))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
		Convey("with a host with that does not have a user", func() {
			h := Host{
				Id:        "h",
				Status:    evergreen.HostRunning,
				StartedBy: "anotherUser",
			}
			So(h.Insert(), ShouldBeNil)
			hosts, err := Find(db.Query(AgentLastCommunicationTimeElapsed(now)))
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
			So(h.Insert(), ShouldBeNil)

			hosts, err := Find(NeedsNewAgentFlagSet())
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, h.Id)
		})
		Convey("with a host having no specified bootstrap method marked as needing a new agent", func() {
			h := Host{
				Id:            "h",
				Status:        evergreen.HostRunning,
				StartedBy:     evergreen.User,
				NeedsNewAgent: true,
			}
			So(h.Insert(), ShouldBeNil)

			hosts, err := Find(NeedsNewAgentFlagSet())
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, h.Id)
		})
	})
}

func TestSetNeedsNewAgent(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"SucceedsOnLegacyHosts": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert())

			require.NoError(t, h.SetNeedsNewAgent(true))
			assert.True(t, h.NeedsNewAgent)

			dbHost, err := FindOne(ById(h.Id))
			require.NoError(t, err)
			assert.True(t, dbHost.NeedsNewAgent)
		},
		"NoopsOnNonLegacyHosts": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
			h.Distro.BootstrapSettings.Communication = distro.CommunicationMethodSSH
			require.NoError(t, h.Insert())

			require.NoError(t, h.SetNeedsNewAgent(true))
			assert.False(t, h.NeedsNewAgent)

			dbHost, err := FindOne(ById(h.Id))
			require.NoError(t, err)
			assert.False(t, dbHost.NeedsNewAgent)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t, &Host{Id: "id"})
		})
	}
}

func TestAgentMonitorLastCommunicationTimeElapsed(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"FindsNotRecentlyCommunicatedHosts": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert())

			hosts, err := Find(db.Query(AgentMonitorLastCommunicationTimeElapsed(time.Now())))
			require.NoError(t, err)

			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"DoesNotFindRecentlyCommunicatedHosts": func(t *testing.T, h *Host) {
			h.LastCommunicationTime = time.Now()
			require.NoError(t, h.Insert())

			hosts, err := Find(db.Query(AgentMonitorLastCommunicationTimeElapsed(time.Now())))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
		"DoesNotFindLegacyHosts": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			h.Distro.BootstrapSettings.Communication = distro.CommunicationMethodLegacySSH
			require.NoError(t, h.Insert())

			hosts, err := Find(db.Query(AgentMonitorLastCommunicationTimeElapsed(time.Now())))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
		"DoesNotFindHostsWithoutBootstrapMethod": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = ""
			h.Distro.BootstrapSettings.Communication = ""
			require.NoError(t, h.Insert())

			hosts, err := Find(db.Query(AgentMonitorLastCommunicationTimeElapsed(time.Now())))
			require.NoError(t, err)

			assert.Empty(t, hosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection), "error clearing %s collection", Collection)
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

func TestFindByNeedsNewAgentMonitor(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"NotRunningHost": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostDecommissioned
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"DoesNotNeedNewAgentMonitor": func(t *testing.T, h *Host) {
			h.NeedsNewAgentMonitor = false
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"BootstrapLegacySSH": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodLegacySSH
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
			require.NoError(t, err)
			require.Len(t, hosts, 0)
		},
		"BootstrapSSH": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"BootstrapUserData": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodUserData
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"BootstrapPreconfiguredImage": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodPreconfiguredImage
			require.NoError(t, h.Insert())

			hosts, err := FindByNeedsNewAgentMonitor()
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
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"ReturnsHostsProvisionedButNotRunning": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert())

			hosts, err := FindUserDataSpawnHostsProvisioning()
			require.NoError(t, err)
			require.Len(t, hosts, 1)
			assert.Equal(t, h.Id, hosts[0].Id)
		},
		"IgnoresNonUserDataBootstrap": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert())

			hosts, err := FindUserDataSpawnHostsProvisioning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresUnprovisionedHosts": func(t *testing.T, h *Host) {
			h.Provisioned = false
			require.NoError(t, h.Insert())

			hosts, err := FindUserDataSpawnHostsProvisioning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresHostsSpawnedByEvergreen": func(t *testing.T, h *Host) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert())

			hosts, err := FindUserDataSpawnHostsProvisioning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
		"IgnoresRunningSpawnHosts": func(t *testing.T, h *Host) {
			h.Status = evergreen.HostRunning
			require.NoError(t, h.Insert())

			hosts, err := FindUserDataSpawnHostsProvisioning()
			require.NoError(t, err)
			assert.Empty(t, hosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection), "error clearing %s collection", Collection)
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := Host{
				Id:          "host_id",
				Status:      evergreen.HostProvisioning,
				Provisioned: true,
				StartedBy:   "user",
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

func TestFindByExpiringJasperCredentials(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
		"IgnoresLegacyHost": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostRunning,
			}
			require.NoError(t, h.Insert())

			creds, err := h.GenerateJasperCredentials(ctx, env)
			require.NoError(t, err)
			require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))

			dbHosts, err := FindByExpiringJasperCredentials(time.Duration(math.MaxInt64))
			require.NoError(t, err)
			assert.Empty(t, dbHosts)
		},
		"IgnoresWithoutCredentials": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostRunning,
			}
			require.NoError(t, h.Insert())

			dbHosts, err := FindByExpiringJasperCredentials(time.Duration(math.MaxInt64))
			require.NoError(t, err)
			assert.Empty(t, dbHosts)
		},
		"IgnoresNonexpiringCredentials": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostRunning,
			}
			require.NoError(t, h.Insert())

			creds, err := h.GenerateJasperCredentials(ctx, env)
			require.NoError(t, err)
			require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))

			dbHosts, err := FindByExpiringJasperCredentials(time.Second)
			require.NoError(t, err)
			assert.Empty(t, dbHosts)
		},
		"ReturnsWithExpiringCredentials": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostRunning,
			}
			require.NoError(t, h.Insert())

			creds, err := h.GenerateJasperCredentials(ctx, env)
			require.NoError(t, err)
			require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))

			dbHosts, err := FindByExpiringJasperCredentials(time.Duration(math.MaxInt64))
			require.NoError(t, err)
			require.Len(t, dbHosts, 1)
			assert.Equal(t, h.Id, dbHosts[0].Id)
		},
		"IgnoresNotRunning": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostTerminated,
			}
			require.NoError(t, h.Insert())

			creds, err := h.GenerateJasperCredentials(ctx, env)
			require.NoError(t, err)
			require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))

			dbHosts, err := FindByExpiringJasperCredentials(time.Duration(math.MaxInt64))
			require.NoError(t, err)
			assert.Empty(t, dbHosts)
		},
		"IgnoresContainers": func(ctx context.Context, t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				JasperCredentialsID: "cid",
				Status:              evergreen.HostRunning,
				ParentID:            "parent",
			}
			require.NoError(t, h.Insert())

			creds, err := h.GenerateJasperCredentials(ctx, env)
			require.NoError(t, err)
			require.NoError(t, h.SaveJasperCredentials(ctx, env, creds))

			dbHosts, err := FindByExpiringJasperCredentials(time.Duration(math.MaxInt64))
			require.NoError(t, err)
			assert.Empty(t, dbHosts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx, "", nil))
			env.EnvContext = tctx

			require.NoError(t, setupCredentialsCollection(ctx, env))
			testCase(tctx, t)
			assert.NoError(t, db.ClearCollections(evergreen.CredentialsCollection, Collection))
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

	assert.InDelta(int64(10*time.Minute), int64(hostThatRanTask.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(1*time.Minute), int64(hostThatJustStarted.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(15*time.Minute), int64(hostWithNoCreateTime.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
	assert.InDelta(int64(7*time.Minute), int64(hostWithOnlyCreateTime.GetElapsedCommunicationTime()), float64(1*time.Millisecond))
}

func TestHostUpsert(t *testing.T) {
	assert := assert.New(t)
	const hostID = "upsertTest"
	testHost := &Host{
		Id:             hostID,
		Host:           "dns",
		User:           "user",
		Distro:         distro.Distro{Id: "distro1"},
		Provisioned:    true,
		StartedBy:      "started_by",
		ExpirationTime: time.Now().Round(time.Second),
		Provider:       "provider",
		Tag:            "tag",
		InstanceType:   "instance",
		Zone:           "zone",
		Project:        "project",
		ProvisionOptions: &ProvisionOptions{
			LoadCLI: true,
			TaskId:  "task_id",
		},
		ContainerImages: map[string]bool{},
	}

	// test inserting new host
	_, err := testHost.Upsert()
	assert.NoError(err)
	hostFromDB, err := FindOne(ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(testHost, hostFromDB)

	// test updating the same host
	testHost.User = "user2"
	_, err = testHost.Upsert()
	assert.NoError(err)
	hostFromDB, err = FindOne(ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.Equal(testHost.User, hostFromDB.User)

	// test updating a field that is not upserted
	testHost.Secret = "secret"
	_, err = testHost.Upsert()
	assert.NoError(err)
	hostFromDB, err = FindOne(ById(hostID))
	assert.NoError(err)
	assert.NotNil(hostFromDB)
	assert.NotEqual(testHost.Secret, hostFromDB.Secret)
}

func TestHostStats(t *testing.T) {
	assert := assert.New(t)

	const d1 = "distro1"
	const d2 = "distro2"

	require.NoError(t, db.Clear(Collection), "error clearing hosts collection")
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

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
	require.NoError(t, db.ClearCollections(Collection, task.Collection), "error clearing collections")
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())

	hosts, err := FindRunningHosts(true)
	assert.NoError(err)

	assert.Equal(3, len(hosts))
	assert.Equal(task1.Id, hosts[0].RunningTaskFull.Id)
	assert.Equal(task2.Id, hosts[1].RunningTaskFull.Id)
	assert.Nil(hosts[2].RunningTaskFull)
}

func TestInactiveHostCountPipeline(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection), "error clearing collections")
	assert := assert.New(t)

	h1 := Host{
		Id:       "h1",
		Status:   evergreen.HostRunning,
		Provider: evergreen.HostTypeStatic,
	}
	assert.NoError(h1.Insert())
	h2 := Host{
		Id:       "h2",
		Status:   evergreen.HostQuarantined,
		Provider: evergreen.HostTypeStatic,
	}
	assert.NoError(h2.Insert())
	h3 := Host{
		Id:       "h3",
		Status:   evergreen.HostDecommissioned,
		Provider: "notstatic",
	}
	assert.NoError(h3.Insert())
	h4 := Host{
		Id:       "h4",
		Status:   evergreen.HostRunning,
		Provider: "notstatic",
	}
	assert.NoError(h4.Insert())
	h5 := Host{
		Id:       "h5",
		Status:   evergreen.HostQuarantined,
		Provider: "notstatic",
	}
	assert.NoError(h5.Insert())

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

func TestIdleEphemeralGroupedByDistroID(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	const d1 = "distro1"
	const d2 = "distro2"

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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())

	idleHostsByDistroID, err := IdleEphemeralGroupedByDistroID()
	assert.NoError(err)
	assert.Equal(2, len(idleHostsByDistroID))

	// Confirm the hosts are sorted from oldest to newest CreationTime.
	if idleHostsByDistroID[0].DistroID == d2 {
		assert.Equal(2, len(idleHostsByDistroID[0].IdleHosts))
		assert.Equal("host5", idleHostsByDistroID[0].IdleHosts[0].Id)
		assert.Equal("host3", idleHostsByDistroID[0].IdleHosts[1].Id)
		assert.Equal(3, len(idleHostsByDistroID[1].IdleHosts))
		assert.Equal("host4", idleHostsByDistroID[1].IdleHosts[0].Id)
		assert.Equal("host1", idleHostsByDistroID[1].IdleHosts[1].Id)
		assert.Equal("host2", idleHostsByDistroID[1].IdleHosts[2].Id)
	} else {
		assert.Equal(3, len(idleHostsByDistroID[0].IdleHosts))
		assert.Equal("host4", idleHostsByDistroID[0].IdleHosts[0].Id)
		assert.Equal("host1", idleHostsByDistroID[0].IdleHosts[1].Id)
		assert.Equal("host2", idleHostsByDistroID[0].IdleHosts[2].Id)
		assert.Equal(2, len(idleHostsByDistroID[1].IdleHosts))
		assert.Equal("host5", idleHostsByDistroID[1].IdleHosts[0].Id)
		assert.Equal("host3", idleHostsByDistroID[1].IdleHosts[1].Id)
	}
}

func TestFindAllRunningContainers(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	containers, err := FindAllRunningContainers()
	assert.NoError(err)
	assert.Equal(2, len(containers))
}

func TestFindAllRunningContainersEmpty(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	containers, err := FindAllRunningContainers()
	assert.NoError(err)
	assert.Empty(containers)
}

func TestFindAllRunningParents(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	hosts, err := FindAllRunningParents()
	assert.NoError(err)
	assert.Equal(3, len(hosts))

}

func TestFindAllRunningParentsOrdered(t *testing.T) {
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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())

	hosts, err := FindAllRunningParentsOrdered()
	assert.NoError(err)
	assert.Equal(hosts[0].Id, host5.Id)
	assert.Equal(hosts[1].Id, host1.Id)
	assert.Equal(hosts[2].Id, host7.Id)
	assert.Equal(hosts[3].Id, host4.Id)

}

func TestFindAllRunningParentsEmpty(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())

	containers, err := FindAllRunningParents()
	assert.NoError(err)
	assert.Empty(containers)
}

func TestGetContainers(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	containers, err := host1.GetContainers()
	assert.NoError(err)
	assert.Equal(5, len(containers))
}

func TestGetContainersNotParent(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	containers, err := host1.GetContainers()
	assert.EqualError(err, "Host does not host containers")
	assert.Empty(containers)
}

func TestIsIdleParent(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())

	// does not have containers --> false
	idle, err := host1.IsIdleParent()
	assert.False(idle)
	assert.NoError(err)

	// recent provision time --> false
	idle, err = host2.IsIdleParent()
	assert.False(idle)
	assert.NoError(err)

	// old provision time --> true
	idle, err = host3.IsIdleParent()
	assert.True(idle)
	assert.NoError(err)

	// has decommissioned container --> false
	idle, err = host4.IsIdleParent()
	assert.False(idle)
	assert.NoError(err)

	// ios a container --> false
	idle, err = host5.IsIdleParent()
	assert.False(idle)
	assert.NoError(err)

}

func TestFindParentOfContainer(t *testing.T) {
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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())

	parent, err := host1.GetParent()
	assert.NoError(err)
	assert.NotNil(parent)
}

func TestFindParentOfContainerNoParent(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	host := &Host{
		Id:     "hostOne",
		Host:   "host",
		User:   "user",
		Distro: distro.Distro{Id: "distro"},
		Status: evergreen.HostRunning,
	}

	assert.NoError(host.Insert())

	parent, err := host.GetParent()
	assert.EqualError(err, "Host does not have a parent")
	assert.Nil(parent)
}

func TestFindParentOfContainerCannotFindParent(t *testing.T) {
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

	assert.NoError(host.Insert())

	parent, err := host.GetParent()
	require.Error(t, err)
	assert.Contains(err.Error(), "not found")
	assert.Nil(parent)
}

func TestFindParentOfContainerNotParent(t *testing.T) {
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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())

	parent, err := host1.GetParent()
	assert.EqualError(err, "Host found is not a parent")
	assert.Nil(parent)
}

func TestLastContainerFinishTimePipeline(t *testing.T) {

	require.NoError(t, db.Clear(Collection), "error clearing %v collections", Collection)
	require.NoError(t, db.Clear(task.Collection), "Error clearing '%v' collection", task.Collection)
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
	assert.NoError(h1.Insert())
	h2 := Host{
		Id:          "h2",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t2",
	}
	assert.NoError(h2.Insert())
	h3 := Host{
		Id:          "h3",
		Status:      evergreen.HostRunning,
		ParentID:    "p1",
		RunningTask: "t3",
	}
	assert.NoError(h3.Insert())
	h4 := Host{
		Id:          "h4",
		Status:      evergreen.HostRunning,
		RunningTask: "t4",
	}
	assert.NoError(h4.Insert())
	h5 := Host{
		Id:          "h5",
		Status:      evergreen.HostRunning,
		ParentID:    "p2",
		RunningTask: "t5",
	}
	assert.NoError(h5.Insert())
	h6 := Host{
		Id:          "h6",
		Status:      evergreen.HostRunning,
		ParentID:    "p2",
		RunningTask: "t6",
	}
	assert.NoError(h6.Insert())
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
	assert := assert.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(Collection))

	hosts := []*Host{
		{
			Id:     "1",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TaskID:  "task_1",
				BuildID: "build_1",
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
				TaskID:  "task_2",
				BuildID: "build_1",
			},
		},
		{
			Id:     "5",
			Status: evergreen.HostDecommissioned,
			SpawnOptions: SpawnOptions{
				TaskID:  "task_1",
				BuildID: "build_1",
			},
		},
		{
			Id:     "6",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TaskID:  "task_2",
				BuildID: "build_1",
			},
		},
	}
	for i := range hosts {
		require.NoError(hosts[i].Insert())
	}
	found, err := FindAllHostsSpawnedByTasks()
	assert.NoError(err)
	assert.Len(found, 2)
	assert.Equal(found[0].Id, "1")
	assert.Equal(found[1].Id, "4")

	found, err = FindHostsSpawnedByTask("task_1")
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal(found[0].Id, "1")

	found, err = FindHostsSpawnedByBuild("build_1")
	assert.NoError(err)
	assert.Len(found, 2)
	assert.Equal(found[0].Id, "1")
	assert.Equal(found[1].Id, "4")
}

func TestCountContainersOnParents(t *testing.T) {
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
	assert.NoError(h1.Insert())
	assert.NoError(h2.Insert())
	assert.NoError(h3.Insert())
	assert.NoError(h4.Insert())
	assert.NoError(h5.Insert())
	assert.NoError(h6.Insert())

	c1, err := HostGroup{h1, h2}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c1, 3)

	c2, err := HostGroup{h1, h3}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c2, 2)

	c3, err := HostGroup{h2, h3}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c3, 1)

	// Parents have no containers
	c4, err := HostGroup{h3}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c4, 0)

	// Parents are actually containers
	c5, err := HostGroup{h4, h5, h6}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c5, 0)

	// Parents list is empty
	c6, err := HostGroup{}.CountContainersOnParents()
	assert.NoError(err)
	assert.Equal(c6, 0)
}

func TestFindUphostContainersOnParents(t *testing.T) {
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
	assert.NoError(h1.Insert())
	assert.NoError(h2.Insert())
	assert.NoError(h3.Insert())
	assert.NoError(h4.Insert())
	assert.NoError(h5.Insert())
	assert.NoError(h6.Insert())

	hosts1, err := HostGroup{h1, h2, h3}.FindUphostContainersOnParents()
	assert.NoError(err)
	assert.Equal([]Host{h4, h5}, hosts1)

	// Parents have no containers
	hosts2, err := HostGroup{h3}.FindUphostContainersOnParents()
	assert.NoError(err)
	assert.Empty(hosts2)

	// Parents are actually containers
	hosts3, err := HostGroup{h4, h5, h6}.FindUphostContainersOnParents()
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

func TestFindAllRunningParentsByDistro(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(host5.Insert())
	assert.NoError(host6.Insert())
	assert.NoError(host7.Insert())
	assert.NoError(host8.Insert())

	parents, err := FindAllRunningParentsByDistro(d1)
	assert.NoError(err)
	assert.Equal(2, len(parents))
}

func TestFindUphostParentsByContainerPool(t *testing.T) {
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
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	hosts, err := findUphostParentsByContainerPool("test-pool")
	assert.NoError(err)
	assert.Equal([]Host{*host1}, hosts)

	hosts, err = findUphostParentsByContainerPool("missing-test-pool")
	assert.NoError(err)
	assert.Empty(hosts)

}

func TestHostsSpawnedByTasks(t *testing.T) {
	assert := require.New(t)
	require := require.New(t)
	require.NoError(db.ClearCollections(Collection, task.Collection, build.Collection))
	finishedTask := &task.Task{
		Id:     "running_task",
		Status: evergreen.TaskSucceeded,
	}
	require.NoError(finishedTask.Insert())
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
		require.NoError(hosts[i].Insert())
	}

	found, err := allHostsSpawnedByTasksTimedOut()
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal("running_host_timeout", found[0].Id)

	found, err = allHostsSpawnedByFinishedTasks()
	assert.NoError(err)
	assert.Len(found, 2)
	should := map[string]bool{
		"running_host_task":          false,
		"running_host_task_building": false,
	}
	for _, f := range found {
		should[f.Id] = true
	}
	for k, v := range should {
		assert.True(v, fmt.Sprintf("failed to find host %s", k))
	}

	found, err = allHostsSpawnedByFinishedBuilds()
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

	found, err = AllHostsSpawnedByTasksToTerminate()
	assert.NoError(err)
	assert.Len(found, 5)
	should = map[string]bool{
		"running_host_timeout":        false,
		"running_host_task":           false,
		"running_host_task_building":  false,
		"running_host_build":          false,
		"running_host_build_starting": false,
	}
	for _, f := range found {
		should[f.Id] = true
	}
	for k, v := range should {
		assert.True(v, fmt.Sprintf("failed to find host %s", k))
	}
}

func TestFindByFirstProvisioningAttempt(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection))

	hosts := []Host{
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
			Id:                "host4",
			ProvisionAttempts: 3,
			Status:            evergreen.HostProvisioning,
		},
	}
	for i := range hosts {
		assert.NoError(hosts[i].Insert())
	}

	hosts, err := FindByFirstProvisioningAttempt()
	assert.NoError(err)
	assert.Len(hosts, 1)
	assert.Equal("host3", hosts[0].Id)

	assert.NoError(db.ClearCollections(Collection))
	assert.NoError(db.Insert(Collection, bson.M{
		"_id":    "host5",
		"status": evergreen.HostProvisioning,
	}))
	hosts, err = FindByFirstProvisioningAttempt()
	assert.NoError(err)
	assert.Empty(hosts)
}

func TestCountContainersRunningAtTime(t *testing.T) {
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
		assert.NoError(containers[i].Insert())
	}

	count1, err := parent.CountContainersRunningAtTime(now.Add(-15 * time.Minute))
	assert.NoError(err)
	assert.Equal(0, count1)

	count2, err := parent.CountContainersRunningAtTime(now)
	assert.NoError(err)
	assert.Equal(2, count2)

	count3, err := parent.CountContainersRunningAtTime(now.Add(15 * time.Minute))
	assert.NoError(err)
	assert.Equal(2, count3)
}

func TestEstimateNumContainersForDuration(t *testing.T) {
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
		assert.NoError(containers[i].Insert())
	}

	estimate1, err := parent.EstimateNumContainersForDuration(now.Add(-15*time.Minute), now)
	assert.NoError(err)
	assert.Equal(1.0, estimate1)

	estimate2, err := parent.EstimateNumContainersForDuration(now, now.Add(15*time.Minute))
	assert.NoError(err)
	assert.Equal(2.0, estimate2)
}

func TestFindTerminatedHostsRunningTasksQuery(t *testing.T) {
	t.Run("QueryExecutesProperly", func(t *testing.T) {
		hosts, err := FindTerminatedHostsRunningTasks()
		assert.NoError(t, err)
		assert.Len(t, hosts, 0)
	})
	t.Run("QueryFindsResults", func(t *testing.T) {
		h := Host{
			Id:          "bar",
			RunningTask: "foo",
			Status:      evergreen.HostTerminated,
		}
		assert.NoError(t, h.Insert())

		hosts, err := FindTerminatedHostsRunningTasks()
		assert.NoError(t, err)
		if assert.Len(t, hosts, 1) {
			assert.Equal(t, h.Id, hosts[0].Id)
		}
	})
}

func TestFindUphostParents(t *testing.T) {
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

	assert.NoError(h1.Insert())
	assert.NoError(h2.Insert())
	assert.NoError(h3.Insert())
	assert.NoError(h4.Insert())
	assert.NoError(h5.Insert())

	uphostParents, err := findUphostParentsByContainerPool("test-pool")
	assert.NoError(err)
	assert.Equal(2, len(uphostParents))
}

func TestRemoveStaleInitializing(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	assert.NoError(db.Clear(Collection))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := testutil.NewEnvironment(ctx, t)

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
			Provider:     evergreen.ProviderNameEc2Auto,
		},
		{
			Id:           "host2",
			Distro:       distro1,
			Status:       evergreen.HostUninitialized,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Auto,
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
			Provider:     evergreen.ProviderNameEc2Auto,
		},
		{
			Id:           "host5",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-5 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Auto,
		},
		{
			Id:           "host6",
			Distro:       distro1,
			Status:       evergreen.HostBuilding,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Auto,
		},
		{
			Id:           "host7",
			Distro:       distro2,
			Status:       evergreen.HostRunning,
			CreationTime: now.Add(-30 * time.Minute),
			UserHost:     false,
			Provider:     evergreen.ProviderNameEc2Auto,
		},
	}

	for i, _ := range hosts {
		require.NoError(hosts[i].Insert())
		creds, err := hosts[i].GenerateJasperCredentials(ctx, env)
		require.NoError(err)
		require.NoError(hosts[i].SaveJasperCredentials(ctx, env, creds))
	}

	err := RemoveStaleInitializing(distro1.Id)
	assert.NoError(err)

	numHosts, err := Count(All)
	assert.NoError(err)
	assert.Equal(5, numHosts)

	dbCreds := certdepot.User{}
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host1"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host3"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host4"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host5"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host7"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.True(adb.ResultsNotFound(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host2"}, db.NoProjection, db.NoSort, &dbCreds)))
	assert.True(adb.ResultsNotFound(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host6"}, db.NoProjection, db.NoSort, &dbCreds)))

	err = RemoveStaleInitializing(distro2.Id)
	assert.NoError(err)
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host1"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host3"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host5"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.NoError(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host7"}, db.NoProjection, db.NoSort, &dbCreds))
	assert.True(adb.ResultsNotFound(db.FindOne(evergreen.CredentialsCollection, bson.M{CertUserIDKey: "host4"}, db.NoProjection, db.NoSort, &dbCreds)))

	numHosts, err = Count(All)
	assert.NoError(err)
	assert.Equal(4, numHosts)

}

func TestStaleRunningTasks(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, task.Collection))
	h1 := Host{
		Id:          "h1",
		RunningTask: "t1",
		Status:      evergreen.HostRunning,
	}
	assert.NoError(h1.Insert())
	h2 := Host{
		Id:          "h2",
		RunningTask: "t2",
		Status:      evergreen.HostRunning,
	}
	assert.NoError(h2.Insert())
	h3 := Host{
		Id:          "h3",
		RunningTask: "t3",
		Status:      evergreen.HostRunning,
	}
	assert.NoError(h3.Insert())
	t1 := task.Task{
		Id:            "t1",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: time.Now().Add(-15 * time.Minute),
	}
	assert.NoError(t1.Insert())
	t2 := task.Task{
		Id:            "t2",
		Status:        evergreen.TaskDispatched,
		LastHeartbeat: time.Now().Add(-25 * time.Minute),
	}
	assert.NoError(t2.Insert())
	t3 := task.Task{
		Id:            "t3",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: time.Now().Add(-1 * time.Minute),
	}
	assert.NoError(t3.Insert())

	tasks, err := FindStaleRunningTasks(10 * time.Minute)
	assert.NoError(err)
	assert.Len(tasks, 2)
}

func TestStaleRunningTasksAgg(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, task.Collection))
	now := time.Now()
	staleness := 5 * time.Minute

	staleTask := &task.Task{
		Id:            "stale",
		Status:        evergreen.TaskStarted,
		Execution:     2,
		LastHeartbeat: now.Add(-2 * staleness),
		HostId:        "staleHost",
	}
	assert.NoError(staleTask.Insert())
	staleHost := &Host{
		Id:          "staleHost",
		RunningTask: staleTask.Id,
	}
	assert.NoError(staleHost.Insert())
	unstaleTask := &task.Task{
		Id:            "unstale",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: now.Add(-1 * time.Second),
		HostId:        "unstaleHost",
	}
	assert.NoError(unstaleTask.Insert())
	unstaleHost := &Host{
		Id:          "unstaleHost",
		RunningTask: unstaleTask.Id,
	}
	assert.NoError(unstaleHost.Insert())
	// task assigned to host that is running teardown_group of the previous task
	unrelatedTask := &task.Task{
		Id:            "unrelatedTask",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: now.Add(-2 * staleness),
		HostId:        "teardownGroupHost",
	}
	assert.NoError(unrelatedTask.Insert())

	// task assigned to host that is running teardown_group of the previous task, but is not timed out
	task3 := task.Task{
		Id:            "task3",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: now.Add(-2 * staleness),
		HostId:        "teardownGroupHost",
	}
	assert.NoError(task3.Insert())

	teardownGroupHost := Host{
		Id:                     "teardownGroupHost",
		RunningTask:            task3.Id,
		RunningTeardownForTask: "somethingelse",
		RunningTeardownSince:   time.Now().Add(-1 * time.Minute),
	}
	assert.NoError(teardownGroupHost.Insert())
	// task assigned to host that is running teardown_group of the previous task that has timed out
	task4 := task.Task{
		Id:            "task4",
		Status:        evergreen.TaskStarted,
		LastHeartbeat: now.Add(-2 * staleness),
		HostId:        "teardownGroupHost2",
	}
	assert.NoError(task4.Insert())
	teardownGroupHost2 := Host{
		Id:                     "teardownGroupHost2",
		RunningTask:            task4.Id,
		RunningTeardownForTask: "somethingelse",
		RunningTeardownSince:   time.Now().Add(-40 * time.Minute),
	}
	assert.NoError(teardownGroupHost2.Insert())

	tasks, err := StaleRunningTaskIDs(staleness)
	assert.NoError(err)
	assert.Len(tasks, 2)
	assert.Equal(staleTask.Id, tasks[0].Id)
	assert.Equal(staleTask.Execution, tasks[0].Execution)
	assert.Equal(task4.Id, tasks[1].Id)
	assert.Equal(task4.Execution, tasks[1].Execution)
}

func TestNumNewParentsNeeded(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())

	existingParents, err := findUphostParentsByContainerPool(d.ContainerPool)
	assert.NoError(err)
	assert.Len(existingParents, 2)
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents()
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
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
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

	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	existingParents, err := findUphostParentsByContainerPool(d.ContainerPool)
	assert.NoError(err)
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents()
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
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock,
		ContainerPool: "test-pool"}
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
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(task1.Insert())
	assert.NoError(task2.Insert())

	availableParent, err := GetNumContainersOnParents(d)
	assert.NoError(err)

	assert.Equal(2, len(availableParent))
}

func TestFindNoAvailableParent(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}
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
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())
	assert.NoError(host4.Insert())
	assert.NoError(task1.Insert())
	assert.NoError(task2.Insert())

	availableParent, err := GetNumContainersOnParents(d)
	assert.NoError(err)
	assert.Equal(0, len(availableParent))
}

func TestGetNumNewParentsAndHostsToSpawn(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 3, Provider: evergreen.ProviderNameMock}
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
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(host2.Insert())
	assert.NoError(host3.Insert())

	parents, hosts, err := getNumNewParentsAndHostsToSpawn(pool, 3, false)
	assert.NoError(err)
	assert.Equal(1, parents) // need two parents, but can only spawn 1
	assert.Equal(2, hosts)

	parents, hosts, err = getNumNewParentsAndHostsToSpawn(pool, 3, true)
	assert.NoError(err)
	assert.Equal(2, parents)
	assert.Equal(3, hosts)
}

func TestGetNumNewParentsWithInitializingParentAndHost(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections("hosts", "distro", "tasks"))

	d := distro.Distro{Id: "distro", PoolSize: 2, Provider: evergreen.ProviderNameMock}
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
	assert.NoError(d.Insert())
	assert.NoError(host1.Insert())
	assert.NoError(container.Insert())

	parents, hosts, err := getNumNewParentsAndHostsToSpawn(pool, 4, false)
	assert.NoError(err)
	assert.Equal(1, parents) // need two parents, but can only spawn 1
	assert.Equal(3, hosts)   // should consider the uninitialized container as taking up capacity

	parents, hosts, err = getNumNewParentsAndHostsToSpawn(pool, 4, true)
	assert.NoError(err)
	assert.Equal(2, parents)
	assert.Equal(4, hosts)
}

func TestFindOneByJasperCredentialsID(t *testing.T) {
	id := "id"
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"FailsWithoutJasperCredentialsID": func(t *testing.T, h *Host) {
			h.Id = id
			dbHost, err := FindOneByJasperCredentialsID(id)
			assert.Error(t, err)
			assert.Nil(t, dbHost)
		},
		"FindsHostWithJasperCredentialsID": func(t *testing.T, h *Host) {
			h.JasperCredentialsID = id
			require.NoError(t, h.Insert())
			dbHost, err := FindOneByJasperCredentialsID(id)
			require.NoError(t, err)
			assert.Equal(t, dbHost, h)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t, &Host{})
		})
	}
}

func TestAddTags(t *testing.T) {
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			Tag{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
			Tag{Key: "key-1", Value: "val-1", CanBeModified: true},
			Tag{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	tagsToAdd := []Tag{
		Tag{Key: "key-fixed", Value: "val-new", CanBeModified: false},
		Tag{Key: "key-2", Value: "val-new", CanBeModified: true},
		Tag{Key: "key-3", Value: "val-3", CanBeModified: true},
	}
	h.AddTags(tagsToAdd)
	assert.Equal(t, []Tag{
		Tag{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
		Tag{Key: "key-1", Value: "val-1", CanBeModified: true},
		Tag{Key: "key-2", Value: "val-new", CanBeModified: true},
		Tag{Key: "key-3", Value: "val-3", CanBeModified: true},
	}, h.InstanceTags)
}

func TestDeleteTags(t *testing.T) {
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			Tag{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
			Tag{Key: "key-1", Value: "val-1", CanBeModified: true},
			Tag{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	tagsToDelete := []string{"key-fixed", "key-1"}
	h.DeleteTags(tagsToDelete)
	assert.Equal(t, []Tag{
		Tag{Key: "key-fixed", Value: "val-fixed", CanBeModified: false},
		Tag{Key: "key-2", Value: "val-2", CanBeModified: true},
	}, h.InstanceTags)
}

func TestSetTags(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id: "id",
		InstanceTags: []Tag{
			Tag{Key: "key-1", Value: "val-1", CanBeModified: true},
			Tag{Key: "key-2", Value: "val-2", CanBeModified: true},
		},
	}
	assert.NoError(t, h.Insert())
	h.InstanceTags = []Tag{
		Tag{Key: "key-3", Value: "val-3", CanBeModified: true},
	}
	assert.NoError(t, h.SetTags())
	foundHost, err := FindOneId(h.Id)
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
		assert.EqualError(t, err, fmt.Sprintf("problem parsing tag '%s'", badTag))
	})
	t.Run("LongKey", func(t *testing.T) {
		badKey := strings.Repeat("a", 129)
		tagSlice := []string{"key1=value", fmt.Sprintf("%s=value2", badKey)}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("key '%s' is longer than 128 characters", badKey))
	})
	t.Run("LongValue", func(t *testing.T) {
		badValue := strings.Repeat("a", 257)
		tagSlice := []string{"key1=value2", fmt.Sprintf("key2=%s", badValue)}
		tags, err := MakeHostTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("value '%s' is longer than 256 characters", badValue))
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
	assert.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id:           "id",
		InstanceType: "old-instance-type",
	}
	assert.NoError(t, h.Insert())
	newInstanceType := "new-instance-type"
	assert.NoError(t, h.SetInstanceType(newInstanceType))
	foundHost, err := FindOneId(h.Id)
	assert.NoError(t, err)
	assert.Equal(t, newInstanceType, foundHost.InstanceType)
}

func TestCountSpawnhostsWithNoExpirationByUser(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
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
		assert.NoError(t, h.Insert())
	}
	count, err := CountSpawnhostsWithNoExpirationByUser("user-1")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)
	count, err = CountSpawnhostsWithNoExpirationByUser("user-2")
	assert.NoError(t, err)
	assert.Equal(t, 2, count)
	count, err = CountSpawnhostsWithNoExpirationByUser("user-3")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestFindSpawnhostsWithNoExpirationToExtend(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
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
		assert.NoError(t, h.Insert())
	}

	foundHosts, err := FindSpawnhostsWithNoExpirationToExtend()
	assert.NoError(t, err)
	assert.Len(t, foundHosts, 1)
	assert.Equal(t, "host-1", foundHosts[0].Id)
}

func TestAddVolumeToHost(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	h := &Host{
		Id: "host-1",
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
		},
	}
	assert.NoError(t, h.Insert())

	newAttachment := &VolumeAttachment{
		VolumeID:   "volume-2",
		DeviceName: "device-2",
	}
	assert.NoError(t, h.AddVolumeToHost(newAttachment))
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
	foundHost, err := FindOneId("host-1")
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

func TestRemoveVolumeFromHost(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
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
	assert.NoError(t, h.Insert())
	assert.NoError(t, h.RemoveVolumeFromHost("volume-2"))
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, h.Volumes)
	foundHost, err := FindOneId("host-1")
	assert.NoError(t, err)
	assert.Equal(t, []VolumeAttachment{
		{
			VolumeID:   "volume-1",
			DeviceName: "device-1",
		},
	}, foundHost.Volumes)
}

func TestFindHostWithVolume(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	h := Host{
		Id: "host-1",
		Volumes: []VolumeAttachment{
			{
				VolumeID:   "volume-1",
				DeviceName: "device-1",
			},
		},
	}
	assert.NoError(t, h.Insert())
	foundHost, err := FindHostWithVolume("volume-1")
	assert.NoError(t, err)
	assert.NotNil(t, foundHost)
	foundHost, err = FindHostWithVolume("volume-2")
	assert.NoError(t, err)
	assert.Nil(t, foundHost)
}
