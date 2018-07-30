package host

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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
			testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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
			testutil.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(), ShouldBeNil)

		Convey("setting the host as terminated should set the status and the"+
			" termination time in both the in-memory and database copies of"+
			" the host", func() {

			So(host.Terminate(evergreen.User), ShouldBeNil)
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

func TestMarkAsProvisioned(t *testing.T) {

	Convey("With a host", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		var err error

		host := &Host{
			Id: "hostOne",
		}

		So(host.Insert(), ShouldBeNil)

		Convey("marking the host as provisioned should update the status,"+
			" provisioned, and host name fields in both the in-memory and"+
			" database copies of the host", func() {

			So(host.MarkAsProvisioned(), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			host, err = FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

		})

	})
}

func TestHostCreateSecret(t *testing.T) {
	Convey("With a host with no secret", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t,
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

func TestHostSetExpirationTime(t *testing.T) {

	Convey("With a host", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		initialExpirationTime := time.Now()
		notifications := make(map[string]bool)
		notifications["2h"] = true

		memHost := &Host{
			Id:             "hostOne",
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)
		oldTaskId := "oldId"
		newTaskId := "newId"
		h := Host{
			Id:          "test",
			RunningTask: oldTaskId,
			Status:      evergreen.HostRunning,
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
	})
}

func TestUpsert(t *testing.T) {

	Convey("With a host", t, func() {

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

			testutil.HandleTestingErr(hostWithDistroA.Insert(), t, "Error inserting"+
				"host into database")
			testutil.HandleTestingErr(hostWithDistroB.Insert(), t, "Error inserting"+
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
		testutil.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)
		now := time.Now()
		Convey("with a host that has no last communication time", func() {
			h := Host{
				Id:        "id",
				Status:    evergreen.HostRunning,
				StartedBy: evergreen.User,
			}
			So(h.Insert(), ShouldBeNil)
			hosts, err := Find(NeedsNewAgent(time.Now()))
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
				hosts, err := Find(NeedsNewAgent(time.Now()))
				So(err, ShouldBeNil)
				So(len(hosts), ShouldEqual, 1)
				So(hosts[0].Id, ShouldEqual, h.Id)
			})
		})

		Convey("with a host with a last communication time > 10 mins", func() {
			anotherHost := Host{
				Id: "anotherID",
				LastCommunicationTime: now.Add(-time.Duration(20) * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			So(anotherHost.Insert(), ShouldBeNil)
			hosts, err := Find(NeedsNewAgent(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, anotherHost.Id)
		})

		Convey("with a host with a normal LCT", func() {
			anotherHost := Host{
				Id: "testhost",
				LastCommunicationTime: now.Add(time.Duration(5) * time.Minute),
				Status:                evergreen.HostRunning,
				StartedBy:             evergreen.User,
			}
			So(anotherHost.Insert(), ShouldBeNil)
			hosts, err := Find(NeedsNewAgent(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
			Convey("after resetting the LCT", func() {
				So(anotherHost.ResetLastCommunicated(), ShouldBeNil)
				So(anotherHost.LastCommunicationTime, ShouldResemble, time.Unix(0, 0))
				h, err := Find(NeedsNewAgent(now))
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
			hosts, err := Find(NeedsNewAgent(now))
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
			hosts, err := Find(NeedsNewAgent(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 0)
		})
		Convey("with a host marked as needing a new agent", func() {
			h := Host{
				Id:            "h",
				Status:        evergreen.HostRunning,
				StartedBy:     evergreen.User,
				NeedsNewAgent: true,
			}
			So(h.Insert(), ShouldBeNil)
			hosts, err := Find(NeedsNewAgent(now))
			So(err, ShouldBeNil)
			So(len(hosts), ShouldEqual, 1)
			So(hosts[0].Id, ShouldEqual, "h")
		})
	})
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
		Id: "hostWithNoCreateTime",
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

	testutil.HandleTestingErr(db.Clear(Collection), t, "error clearing hosts collection")
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
	testutil.HandleTestingErr(db.ClearCollections(Collection, task.Collection), t, "error clearing collections")
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

	var hosts []Host
	err := db.Aggregate(Collection, QueryWithFullTaskPipeline(
		bson.M{StatusKey: bson.M{"$ne": evergreen.HostTerminated}}),
		&hosts)
	assert.NoError(err)

	assert.Equal(3, len(hosts))
	assert.Equal(task1.Id, hosts[0].RunningTaskFull.Id)
	assert.Equal(task2.Id, hosts[1].RunningTaskFull.Id)
	assert.Nil(hosts[2].RunningTaskFull)
}

func TestInactiveHostCountPipeline(t *testing.T) {
	testutil.HandleTestingErr(db.ClearCollections(Collection), t, "error clearing collections")
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
	assert.EqualError(err, "Parent not found")
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

	testutil.HandleTestingErr(db.Clear(Collection), t, "error clearing %v collections", Collection)
	testutil.HandleTestingErr(db.Clear(task.Collection), t, "Error clearing '%v' collection", task.Collection)
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

func TestFindRunningContainersOnParents(t *testing.T) {
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

	hosts1, err := HostGroup{h1, h2, h3}.FindRunningContainersOnParents()
	assert.NoError(err)
	assert.Equal([]Host{h4, h5}, hosts1)

	// Parents have no containers
	hosts2, err := HostGroup{h3}.FindRunningContainersOnParents()
	assert.NoError(err)
	assert.Empty(hosts2)

	// Parents are actually containers
	hosts3, err := HostGroup{h4, h5, h6}.FindRunningContainersOnParents()
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

func TestFindAllRunningParentsByContainerPool(t *testing.T) {
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

	hosts, err := FindAllRunningParentsByContainerPool("test-pool")
	assert.NoError(err)
	assert.Equal([]Host{*host1}, hosts)

	hosts, err = FindAllRunningParentsByContainerPool("missing-test-pool")
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
			},
		},
		{
			Id:     "running_host_task",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "running_task",
			},
		},
		{
			Id:     "running_host_build",
			Status: evergreen.HostRunning,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				BuildID:         "running_build",
			},
		},
		{
			Id:     "terminated_host_timeout",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(-time.Minute),
			},
		},
		{
			Id:     "terminated_host_task",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				TaskID:          "running_task",
			},
		},
		{
			Id:     "terminated_host_build",
			Status: evergreen.HostTerminated,
			SpawnOptions: SpawnOptions{
				TimeoutTeardown: time.Now().Add(time.Minute),
				BuildID:         "running_build",
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
	assert.Len(found, 1)
	assert.Equal("running_host_task", found[0].Id)

	found, err = allHostsSpawnedByFinishedBuilds()
	assert.NoError(err)
	assert.Len(found, 1)
	assert.Equal("running_host_build", found[0].Id)

	found, err = AllHostsSpawnedByTasksToTerminate()
	assert.NoError(err)
	assert.Len(found, 3)
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

func TestCountUninitializedParents(t *testing.T) {
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
	}
	h3 := Host{
		Id:            "h3",
		Status:        evergreen.HostRunning,
		HasContainers: true,
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

	numUninitializedParents, err := CountUninitializedParents()
	assert.NoError(err)
	assert.Equal(2, numUninitializedParents)
}
