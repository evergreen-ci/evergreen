package host

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
	"testing"
	"time"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
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

	Convey("When finding hosts", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error clearing"+
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

			Convey("the specified projection, sort, skip, and limit should be"+
				" used", func() {

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

func TestUpdatingHostStatus(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the host's status should update both the in-memory"+
			" and database versions of the host", func() {

			So(host.SetStatus(evergreen.HostRunning), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)

		})

		Convey("if the host is terminated, the status update should fail"+
			" with an error", func() {

			So(host.SetStatus(evergreen.HostTerminated), ShouldBeNil)
			So(host.SetStatus(evergreen.HostRunning), ShouldNotBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)

		})

	})

}

func TestSetHostTerminated(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the host as terminated should set the status and the"+
			" termination time in both the in-memory and database copies of"+
			" the host", func() {

			So(host.Terminate(), ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

		})

	})
}

func TestHostSetDNSName(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the hostname should update both the in-memory and"+
			" database copies of the host", func() {

			So(host.SetDNSName("hostname"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			host, err := FindOne(ById(host.Id))
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

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

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

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, evergreen.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

		})

	})
}

func TestHostSetRunningTask(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the running task for the host should set the running"+
			" task and task dispatch time for both the in-memory and database"+
			" copies of the host", func() {

			taskDispatchTime := time.Now()

			So(host.SetRunningTask("taskId", "c", taskDispatchTime), ShouldBeNil)
			So(host.RunningTask, ShouldEqual, "taskId")
			So(host.AgentRevision, ShouldEqual, "c")
			So(host.TaskDispatchTime.Round(time.Second).Equal(
				taskDispatchTime.Round(time.Second)), ShouldBeTrue)

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)
			So(host.RunningTask, ShouldEqual, "taskId")
			So(host.AgentRevision, ShouldEqual, "c")
			So(host.TaskDispatchTime.Round(time.Second).Equal(
				taskDispatchTime.Round(time.Second)), ShouldBeTrue)

		})

	})
}

func TestHostSetExpirationTime(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

func TestFindRunningSpawnedHosts(t *testing.T) {
	testConfig := evergreen.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	util.HandleTestingErr(db.Clear(Collection), t, "Error"+
		" clearing '%v' collection", Collection)

	Convey("With calling FindRunningSpawnedHosts...", t, func() {
		Convey("if there are no spawned hosts, nothing should be returned",
			func() {
				spawnedHosts, err := Find(IsRunningAndSpawned)
				So(err, ShouldBeNil)
				// make sure we only returned no document
				So(len(spawnedHosts), ShouldEqual, 0)

			})

		Convey("if there are spawned hosts, they should be returned", func() {
			host := &Host{}
			host.Id = "spawned-1"
			host.Status = "running"
			host.StartedBy = "user1"
			util.HandleTestingErr(host.Insert(), t, "error from "+
				"FindRunningSpawnedHosts")
			spawnedHosts, err := Find(IsRunningAndSpawned)
			util.HandleTestingErr(err, t, "error from "+
				"FindRunningSpawnedHosts: %v", err)
			// make sure we only returned no document
			So(len(spawnedHosts), ShouldEqual, 1)

		})
	})
}
func TestSetExpirationNotification(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
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

func TestHostClearRunningTask(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id:               "hostOne",
			RunningTask:      "taskId",
			StartedBy:        evergreen.User,
			Status:           evergreen.HostRunning,
			TaskDispatchTime: time.Now(),
			Pid:              "12345",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("host statistics should properly count this host as active"+
			" but not idle", func() {
			count, err := Count(IsActive)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			count, err = Count(IsIdle)
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 0)
		})

		Convey("clearing the running task should clear the running task, pid,"+
			" and task dispatch time fields from both the in-memory and"+
			" database copies of the host", func() {

			So(host.ClearRunningTask(), ShouldBeNil)
			So(host.RunningTask, ShouldEqual, "")
			So(host.TaskDispatchTime.Equal(time.Unix(0, 0)), ShouldBeTrue)
			So(host.Pid, ShouldEqual, "")

			host, err := FindOne(ById(host.Id))
			So(err, ShouldBeNil)

			So(host.RunningTask, ShouldEqual, "")
			So(host.TaskDispatchTime.Equal(time.Unix(0, 0)), ShouldBeTrue)
			So(host.Pid, ShouldEqual, "")

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

func TestUpsert(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(Collection), t, "Error"+
			" clearing '%v' collection", Collection)

		host := &Host{
			Id:     "hostOne",
			Host:   "host",
			User:   "user",
			Distro: distro.Distro{Id: "distro"},
			Status: evergreen.HostRunning,
		}

		Convey("Performing a host upsert should upsert correctly", func() {
			_, err := host.Upsert()
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

				host, err := FindOne(ById(host.Id))
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
	})
}
