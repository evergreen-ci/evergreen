package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
	"testing"
	"time"
)

var (
	hostTestConfig = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(hostTestConfig))
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

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error clearing"+
			" '%v' collection", HostsCollection)

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

				found, err := FindOneHost(
					bson.M{
						HostIdKey: matchingHost.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(found.Id, ShouldEqual, matchingHost.Id)

			})

		})

		Convey("when finding multiple hosts", func() {

			Convey("the hosts matching the query should be returned", func() {

				matchingHostOne := &Host{
					Id:     "matches",
					Distro: "d1",
				}
				So(matchingHostOne.Insert(), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Distro: "d1",
				}
				So(matchingHostTwo.Insert(), ShouldBeNil)

				nonMatchingHost := &Host{
					Id:     "nonMatches",
					Distro: "d2",
				}
				So(nonMatchingHost.Insert(), ShouldBeNil)

				found, err := FindAllHosts(
					bson.M{
						HostDistroKey: "d1",
					},
					db.NoProjection,
					db.NoSort,
					db.NoSkip,
					db.NoLimit,
				)
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
					Distro: "d1",
					Tag:    "2",
				}
				So(matchingHostOne.Insert(), ShouldBeNil)

				matchingHostTwo := &Host{
					Id:     "matchesAlso",
					Host:   "hostTwo",
					Distro: "d1",
					Tag:    "1",
				}
				So(matchingHostTwo.Insert(), ShouldBeNil)

				matchingHostThree := &Host{
					Id:     "stillMatches",
					Host:   "hostThree",
					Distro: "d1",
					Tag:    "3",
				}
				So(matchingHostThree.Insert(), ShouldBeNil)

				// find the hosts, removing the host field from the projection,
				// sorting by tag, skipping one, and limiting to one

				found, err := FindAllHosts(
					bson.M{
						HostDistroKey: "d1",
					},
					bson.M{
						HostDNSKey: 0,
					},
					[]string{HostTagKey},
					1,
					1,
				)
				So(err, ShouldBeNil)
				So(len(found), ShouldEqual, 1)
				So(found[0].Id, ShouldEqual, matchingHostOne.Id)
				So(found[0].Host, ShouldEqual, "") // filtered out in projection

			})

		})

	})

}

func TestHostFindNextTask(t *testing.T) {

	Convey("With a host", t, func() {

		Convey("when finding the next task to be run on the host", func() {

			util.HandleTestingErr(db.ClearCollections(HostsCollection,
				TasksCollection, TaskQueuesCollection), t,
				"Error clearing test collections")

			host := &Host{
				Id:     "hostId",
				Distro: "d1",
			}
			So(host.Insert(), ShouldBeNil)

			Convey("if there is no task queue for the host's distro, no task"+
				" should be returned", func() {

				nextTask, err := host.FindNextTask()
				So(err, ShouldBeNil)
				So(nextTask, ShouldBeNil)

			})

			Convey("if the task queue is empty, no task should be"+
				" returned", func() {

				tQueue := &TaskQueue{
					Distro: host.Distro,
				}
				So(tQueue.Save(), ShouldBeNil)

				nextTask, err := host.FindNextTask()
				So(err, ShouldBeNil)
				So(nextTask, ShouldBeNil)

			})

			Convey("if the task queue is not empty, the corresponding task"+
				" object from the database should be returned", func() {

				tQueue := &TaskQueue{
					Distro: host.Distro,
					Queue: []TaskQueueItem{
						TaskQueueItem{
							Id: "taskOne",
						},
					},
				}
				So(tQueue.Save(), ShouldBeNil)

				task := &Task{
					Id: "taskOne",
				}
				So(task.Insert(), ShouldBeNil)

				nextTask, err := host.FindNextTask()
				So(err, ShouldBeNil)
				So(nextTask.Id, ShouldEqual, task.Id)

			})

		})

	})
}

func TestUpdatingHostStatus(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the host's status should update both the in-memory"+
			" and database versions of the host", func() {

			So(host.SetStatus(mci.HostRunning), ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)

		})

		Convey("if the host is terminated, the status update should fail"+
			" with an error", func() {

			So(host.SetStatus(mci.HostTerminated), ShouldBeNil)
			So(host.SetStatus(mci.HostRunning), ShouldNotBeNil)
			So(host.Status, ShouldEqual, mci.HostTerminated)

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostTerminated)

		})

	})

}

func TestSetHostTerminated(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the host as terminated should set the status and the"+
			" termination time in both the in-memory and database copies of"+
			" the host", func() {

			So(host.Terminate(), ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostTerminated)
			So(host.TerminationTime.IsZero(), ShouldBeFalse)

		})

	})
}

func TestDecommissionInactiveStaticHosts(t *testing.T) {

	Convey("When decommissioning unused static hosts", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error clearing"+
			" '%v' collection", HostsCollection)

		Convey("if a nil slice is passed in, no host(s) should"+
			" be decommissioned in the database", func() {

			inactiveOne := &Host{
				Id:       "inactiveOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			So(DecommissionInactiveStaticHosts(nil), ShouldBeNil)

			found, err := FindAllHostsNoFilter()
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(found[0].Status, ShouldEqual, mci.HostRunning)
			So(found[1].Status, ShouldEqual, mci.HostRunning)

		})

		Convey("if a non-nil slice is passed in, any static hosts with ids not in"+
			" the slice should be removed from the database", func() {

			activeOne := &Host{
				Id:       "activeStaticOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(activeOne.Insert(), ShouldBeNil)

			activeTwo := &Host{
				Id:       "activeStaticTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(activeTwo.Insert(), ShouldBeNil)

			inactiveOne := &Host{
				Id:       "inactiveStaticOne",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveOne.Insert(), ShouldBeNil)

			inactiveTwo := &Host{
				Id:       "inactiveStaticTwo",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeStatic,
			}
			So(inactiveTwo.Insert(), ShouldBeNil)

			inactiveEC2One := &Host{
				Id:       "inactiveEC2One",
				Status:   mci.HostRunning,
				Provider: mci.HostTypeEC2,
			}
			So(inactiveEC2One.Insert(), ShouldBeNil)

			inactiveUnknownTypeOne := &Host{
				Id:     "inactiveUnknownTypeOne",
				Status: mci.HostRunning,
			}
			So(inactiveUnknownTypeOne.Insert(), ShouldBeNil)

			activeStaticHosts := []string{"activeStaticOne", "activeStaticTwo"}
			So(DecommissionInactiveStaticHosts(activeStaticHosts), ShouldBeNil)

			found, err := FindDecommissionedHosts()
			So(err, ShouldBeNil)
			So(len(found), ShouldEqual, 2)
			So(hostIdInSlice(found, inactiveOne.Id), ShouldBeTrue)
			So(hostIdInSlice(found, inactiveTwo.Id), ShouldBeTrue)
		})

	})

}

func TestHostSetDNSName(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("setting the hostname should update both the in-memory and"+
			" database copies of the host", func() {

			So(host.SetDNSName("hostname"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			// if the host is already updated, no new updates should work
			So(host.SetDNSName("hostname2"), ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

			host, err = FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Host, ShouldEqual, "hostname")

		})

	})
}

func TestMarkAsProvisioned(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id: "hostOne",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("marking the host as provisioned should update the status,"+
			" provisioned, and host name fields in both the in-memory and"+
			" database copies of the host", func() {

			So(host.MarkAsProvisioned(), ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)
			So(host.Provisioned, ShouldEqual, true)

		})

	})
}

func TestHostSetRunningTask(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

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

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
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

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

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

			dbHost, err := FindOneHost(
				bson.M{
					HostIdKey: memHost.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
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

			dbHost, err = FindOneHost(
				bson.M{
					HostIdKey: memHost.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
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
	testConfig := mci.TestConfig()
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))

	util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
		" clearing '%v' collection", HostsCollection)

	Convey("With calling FindRunningSpawnedHosts...", t, func() {
		Convey("if there are no spawned hosts, nothing should be returned",
			func() {
				spawnedHosts, err := FindRunningSpawnedHosts()
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
			spawnedHosts, err := FindRunningSpawnedHosts()
			util.HandleTestingErr(err, t, "error from "+
				"FindRunningSpawnedHosts: %v", err)
			// make sure we only returned no document
			So(len(spawnedHosts), ShouldEqual, 1)

		})
	})
}
func TestSetExpirationNotification(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

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

			dbHost, err := FindOneHost(
				bson.M{
					HostIdKey: memHost.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.Notifications, ShouldResemble, notifications)
			So(dbHost.Notifications, ShouldResemble, notifications)

			// now update the expiration notification
			notifications["4h"] = true
			So(memHost.SetExpirationNotification("4h"), ShouldBeNil)
			dbHost, err = FindOneHost(
				bson.M{
					HostIdKey: memHost.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			// ensure the db entries are as expected
			So(err, ShouldBeNil)
			So(memHost.Notifications, ShouldResemble, notifications)
			So(dbHost.Notifications, ShouldResemble, notifications)
		})
	})
}

func TestHostClearRunningTask(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id:               "hostOne",
			RunningTask:      "taskId",
			StartedBy:        mci.MCIUser,
			Status:           mci.HostRunning,
			TaskDispatchTime: time.Now(),
			Pid:              "12345",
		}
		So(host.Insert(), ShouldBeNil)

		Convey("host statistics should properly count this host as active"+
			" but not idle", func() {
			count, err := CountActiveHosts()
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1)
			count, err = CountIdleHosts()
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

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)

			So(host.RunningTask, ShouldEqual, "")
			So(host.TaskDispatchTime.Equal(time.Unix(0, 0)), ShouldBeTrue)
			So(host.Pid, ShouldEqual, "")

			Convey("the count of idle hosts should go up", func() {
				count, err := CountIdleHosts()
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)

				Convey("but the active host count should remain the same", func() {
					count, err = CountActiveHosts()
					So(err, ShouldBeNil)
					So(count, ShouldEqual, 1)
				})
			})

		})

	})
}

func TestUpsert(t *testing.T) {

	Convey("With a host", t, func() {

		util.HandleTestingErr(db.Clear(HostsCollection), t, "Error"+
			" clearing '%v' collection", HostsCollection)

		host := &Host{
			Id:     "hostOne",
			Host:   "host",
			User:   "user",
			Distro: "distro",
			Status: mci.HostRunning,
		}

		Convey("Performing a host upsert should upsert correctly", func() {
			_, err := host.Upsert()
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)

			host, err := FindOneHost(
				bson.M{
					HostIdKey: host.Id,
				},
				db.NoProjection,
				db.NoSort,
			)
			So(err, ShouldBeNil)
			So(host.Status, ShouldEqual, mci.HostRunning)

		})

		Convey("Updating some fields of an already inserted host should cause "+
			"those fields to be updated but should leave status unchanged",
			func() {
				_, err := host.Upsert()
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, mci.HostRunning)

				host, err := FindOneHost(
					bson.M{
						HostIdKey: host.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, mci.HostRunning)
				So(host.Host, ShouldEqual, "host")

				err = UpdateOneHost(
					bson.M{
						HostIdKey: host.Id,
					},
					bson.M{
						"$set": bson.M{
							HostStatusKey: mci.HostDecommissioned,
						},
					},
				)
				So(err, ShouldBeNil)

				// update the hostname and status
				host.Host = "host2"
				host.Status = mci.HostRunning
				_, err = host.Upsert()
				So(err, ShouldBeNil)

				// host db status should remain unchanged
				host, err = FindOneHost(
					bson.M{
						HostIdKey: host.Id,
					},
					db.NoProjection,
					db.NoSort,
				)
				So(err, ShouldBeNil)
				So(host.Status, ShouldEqual, mci.HostDecommissioned)
				So(host.Host, ShouldEqual, "host2")

			})
	})
}
