package cleanup

import (
	"10gen.com/mci"
	db_util "10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"labix.org/v2/mgo/bson"
	"strconv"
	"testing"
	"time"
)

var TestConfig = mci.TestConfig()

func init() {
	db_util.SetGlobalSessionProvider(db_util.SessionFactoryFromConfig(TestConfig))
	if TestConfig.Cleanup.LogFile != "" {
		mci.SetLogger(TestConfig.Cleanup.LogFile)
	}
}

func TestCheckTimeouts(t *testing.T) {
	session, _, err := db_util.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "Error getting connection to db")
	defer session.Close()
	cleanupdb(t)

	Convey("With only non-timed-out tasks running, CheckTimeouts must return empty", t, func() {
		// insert two fake tasks, the first running, the second not running
		util.HandleTestingErr(db_util.Insert(model.TasksCollection, bson.M{
			"_id":            "task1",
			"build_id":       "test",
			"secret":         "secret1",
			"dispatch_time":  time.Now(),
			"start_time":     time.Now(),
			"last_heartbeat": time.Now(),
			"status":         mci.TaskStarted,
			"branch":         "mci-test",
			"host_id":        "host1",
		}), t, "Could not insert test document")
		util.HandleTestingErr(db_util.Insert(model.TasksCollection, bson.M{
			"_id":           "task2",
			"build_id":      "test",
			"branch":        "mci-test",
			"dispatch_time": time.Unix(0, 0),
			"status":        mci.TaskUndispatched,
			"host_id":       "host2",
		}), t, "Could not insert test document")

		// insert two fake hosts
		util.HandleTestingErr(db_util.Insert(model.HostsCollection, bson.M{
			"_id":          "host1",
			"build_id":     "test",
			"host_id":      "host1",
			"host_type":    mci.HostTypeEC2,
			"running_task": "task1",
			"started_by":   mci.MCIUser,
			"distro_id":    "test-distro-one",
		}), t, "Could not insert test document")
		util.HandleTestingErr(db_util.Insert(model.HostsCollection, bson.M{
			"_id":          "host2",
			"build_id":     "test",
			"host_id":      "host2",
			"host_type":    mci.HostTypeEC2,
			"running_task": "",
			"started_by":   mci.MCIUser,
		}), t, "Could not insert test document")

		// insert the referenced build
		util.HandleTestingErr(db_util.Insert(model.HostsCollection, bson.M{
			"_id":          "test",
			"running_task": "task1",
		}), t, "Could not insert test document")

		// now, check timeouts. nothing should be changed because nothing has timed out
		timedOutTasks := CheckTimeouts(TestConfig)
		So(len(timedOutTasks), ShouldEqual, 0)

		Convey("When a task has timed out, CheckTimeouts should return it", func() {
			// update task1 so it has "timed out"
			err := db_util.Update(model.TasksCollection, bson.M{"_id": "task1"}, bson.M{
				"$set": bson.M{"last_heartbeat": time.Now().Add(-TaskTimeOutDuration)},
			})
			util.HandleTestingErr(err, t, "Could not update test doc")
			time.Sleep(1 * time.Millisecond)

			// check again. task1 should have timed out, causing the appropriate fields to
			// be set in the db
			timedOutTasks = CheckTimeouts(TestConfig)
			So(timedOutTasks, ShouldResemble, []string{"task1"})

			// make sure the task's fields were changed appropriately
			task := &model.Task{}
			err = db_util.FindOne(model.TasksCollection, bson.M{"_id": "task1"},
				db_util.NoProjection, db_util.NoSort, task)
			util.HandleTestingErr(err, t, "Could not find test doc")
			So(task.DispatchTime.Equal(time.Unix(0, 0)), ShouldBeTrue)
			So(task.StartTime.Equal(time.Unix(0, 0)), ShouldBeTrue)
			So(task.Secret, ShouldNotEqual, "secret1")

			Convey("The host that ran the task should reflect that it has "+
				"been timed out and cleared", func() {
				// make sure the host's fields were changed appropriately
				host := &model.Host{}
				err = db_util.FindOne(model.HostsCollection, bson.M{"_id": "host1"},
					db_util.NoProjection, db_util.NoSort, host)

				util.HandleTestingErr(err, t, "Could not find test doc")
				So(host.RunningTask, ShouldEqual, "")

				// run one more time, and nothing should change
				timedOutTasks = CheckTimeouts(TestConfig)
				So(timedOutTasks, ShouldResemble, []string{})
			})

		})

	})

}

func TestGetExcessHosts(t *testing.T) {
	cleanupdb(t)
	Convey("With more hosts than MaxHosts on distros, GetExcessHosts should return non-empty", t, func() {
		distros := make(map[string]model.Distro)
		// case in which max hosts is greater than existing hosts
		distros["windows"] = model.Distro{MaxHosts: 2}
		distros["linux"] = model.Distro{MaxHosts: 1}
		_, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		excessHosts, err := GetExcessHosts(distros)
		util.HandleTestingErr(err, t, "error from GetExcessHosts()")
		// make sure we only returned one document
		// any linux host that isn't running a task
		So(len(excessHosts), ShouldEqual, 1)
		for distro, _ := range excessHosts {
			So(distro, ShouldEqual, "linux")
		}
		Convey("When maxhosts is increased to > num hosts, GetExcessHosts returns empty", func() {
			// case in which max hosts is less than existing hosts
			distros["linux"] = model.Distro{MaxHosts: 20}
			excessHosts, err = GetExcessHosts(distros)
			util.HandleTestingErr(err, t, "error from GetExcessHosts()")
			// make sure we only returned NO documents
			So(len(excessHosts), ShouldEqual, 0)
		})
	})
}

func TestGetIdleHosts(t *testing.T) {
	Convey("With batch of test hosts, only one should be returned as 'idle'", t, func() {
		cleanupdb(t)
		currentTime, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		toBeReaped, err := GetIdleHosts(currentTime, allHosts)
		util.HandleTestingErr(err, t, "error from GetIdleHosts()")
		// make sure we only returned one document
		So(len(toBeReaped), ShouldEqual, 1)
		// make sure we only returned THE one document we wanted
		So(toBeReaped[0].Id, ShouldEqual, "reap-1")
	})
}

func TestGetDecommissionedHosts(t *testing.T) {
	Convey("With test hosts, only one should be returned as 'decommissioned'", t, func() {
		cleanupdb(t)
		_, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		toBeReaped, err := GetDecommissionedHosts()
		util.HandleTestingErr(err, t, "error from GetDecommissionedHosts()")
		// make sure we only returned one document
		So(len(toBeReaped), ShouldEqual, 1)
		// make sure we only returned THE one document we wanted
		So(toBeReaped[0].Id, ShouldEqual, "noreap-2")
	})
}

func TestShouldSendWarningNotification(t *testing.T) {
	Convey("A host with no notification sent should trigger the warning", t, func() {
		host := &model.Host{Id: "hostid1"}
		util.HandleTestingErr(host.Insert(), t, "error inserting host")

		So(shouldSendLateProvisionWarning(host), ShouldBeTrue)

		Convey("After SetExpirationNotification, it should no longer trigger the warning", func() {
			//Record the notification as sent so it won't be sent again on next run
			util.HandleTestingErr(host.SetExpirationNotification(LateProvisionWarning),
				t, "Failed to update notifications map for host")
			hostRefreshed, err := model.FindHost(host.Id)
			util.HandleTestingErr(err, t, "Failed to refresh host from database")
			So(shouldSendLateProvisionWarning(hostRefreshed), ShouldBeFalse)
		})
	})
}

func TestShouldSendExpirationNotification(t *testing.T) {
	hostIdOne := "spawned-1"
	warningThresholds := []time.Duration{
		time.Duration(3) * time.Hour,
	}
	expirationTime := time.Now().Add(warningThresholds[0])

	Convey("With calling shouldSendExpirationNotification with a given "+
		"threshold...",
		t, func() {
			util.HandleTestingErr(db_util.Clear(model.HostsCollection), t, "Error "+
				"clearing '%v' collection", model.HostsCollection)

			Convey("if notifications have been sent for all thresholds, "+
				"no further notification should be sent", func() {
				host := &model.Host{}
				host.Id = hostIdOne
				host.ExpirationTime = expirationTime
				host.Notifications = map[string]bool{
					strconv.Itoa(int(warningThresholds[0].Minutes())): true,
				}
				util.HandleTestingErr(host.Insert(), t, "error inserting host")

				shouldSend, _ := shouldSendExpirationNotification(host,
					warningThresholds)
				So(shouldSend, ShouldEqual, false)
			})

			Convey("if no notification has been sent for any thresholds, "+
				"a notification should be sent", func() {
				host := &model.Host{}
				host.Id = hostIdOne
				host.ExpirationTime = expirationTime
				util.HandleTestingErr(host.Insert(), t, "error inserting host")

				shouldSend, threshold := shouldSendExpirationNotification(host,
					warningThresholds)
				So(shouldSend, ShouldEqual, true)
				So(threshold, ShouldEqual, warningThresholds[0])
			})
		})

}

func TestTerminateStaleHosts(t *testing.T) {
	hostIdOne := "spawned-1"
	hostIdTwo := "spawned-2"
	hostIdThree := "spawned-3"
	hostUser := "wisdom"

	Convey("With calling TerminateStaleHosts...", t, func() {
		util.HandleTestingErr(
			db_util.ClearCollections(model.HostsCollection, model.UsersCollection), t, "Error clearing test collection")

		Convey("If there are no expired hosts, nothing should be terminated", func() {
			host := &model.Host{
				Id:             hostIdOne,
				Provider:       "ec2",
				Status:         mci.HostRunning,
				StartedBy:      hostUser,
				ExpirationTime: time.Now().Add(time.Duration(10) * time.Hour),
			}
			util.HandleTestingErr(host.Insert(), t, "error inserting host")

			user := &model.DBUser{Id: host.StartedBy}
			util.HandleTestingErr(user.Insert(), t, "error inserting user")

			staleHosts := TerminateStaleHosts(TestConfig)
			// make sure we didn't terminate any host since the expiration was
			// ahead of now
			So(len(staleHosts), ShouldEqual, 0)

		})

		Convey("if there several spawned hosts that are expired, only "+
			"the expired ones should be terminated", func() {
			host := &model.Host{}

			// host 1 expired
			host.Id = hostIdOne
			host.Status = mci.HostRunning
			host.StartedBy = hostUser
			host.ExpirationTime = time.Now().Add(-time.Duration(2) * time.Hour)
			util.HandleTestingErr(host.Insert(), t, "error inserting host")

			// host 2 expired
			host.Id = hostIdTwo
			host.Status = mci.HostRunning
			host.StartedBy = hostUser
			host.ExpirationTime = time.Now().Add(-time.Duration(1) * time.Hour)
			util.HandleTestingErr(host.Insert(), t, "error inserting host")

			// host 3 unexpired
			host.Id = hostIdThree
			host.Status = mci.HostRunning
			host.StartedBy = hostUser
			host.ExpirationTime = time.Now().Add(time.Duration(1) * time.Hour)
			util.HandleTestingErr(host.Insert(), t, "error inserting host")

			user := &model.DBUser{}
			user.Id = host.StartedBy
			util.HandleTestingErr(user.Insert(), t, "error inserting user")

			staleHosts := TerminateStaleHosts(TestConfig)
			// make sure we didn't terminate any host since the expiration was
			// ahead of now
			So(len(staleHosts), ShouldEqual, 2)

			// ensure we didn't record the unexpired host as stale
			So(staleHosts[0].Id, ShouldNotEqual, hostIdThree)
			So(staleHosts[1].Id, ShouldNotEqual, hostIdThree)
		})
	})
}

func TestGetUnprovisionedHosts(t *testing.T) {
	Convey("With set of test hosts, only 3 should be returned as unprovisioned", t, func() {
		cleanupdb(t)
		currentTime, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		toBeTerminated, err := GetUnprovisionedHosts(currentTime, MaxTimeToMarkAsProvisioned)
		util.HandleTestingErr(err, t, "error from GetUnprovisionedHosts()")
		// make sure we only returned three documents
		So(len(toBeTerminated), ShouldEqual, 3)
	})
}

func TestGetUnproductiveHosts(t *testing.T) {
	Convey("With set of test hosts, only 2 should be returned as unproductive", t, func() {
		cleanupdb(t)
		currentTime, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		toBeTerminated, err := GetUnproductiveHosts(currentTime)
		util.HandleTestingErr(err, t, "error from GetUnproductiveHosts()")
		// make sure we only returned two documents
		So(len(toBeTerminated), ShouldEqual, 2)
	})
}

func TestGetHungHosts(t *testing.T) {
	Convey("With set of test hosts, only 1 should be returned as unproductive", t, func() {
		cleanupdb(t)
		currentTime, allHosts := getInterestingHosts(t)
		for _, host := range allHosts {
			err := host.Insert()
			util.HandleTestingErr(err, t, "could not insert test host")
		}
		hungHosts, err := GetHungHosts(currentTime)
		util.HandleTestingErr(err, t, "error from GetHungHosts()")
		// make sure we only returned one document
		So(len(hungHosts), ShouldEqual, 1)
	})
}

func cleanupdb(t *testing.T) {
	session, _, err := db_util.GetGlobalSessionFactory().GetSession()
	util.HandleTestingErr(err, t, "Error getting connection to db")
	defer session.Close()
	err = db_util.Clear(model.OldTasksCollection)
	util.HandleTestingErr(err, t, "Error removing %v collection ", model.OldTasksCollection)
	err = db_util.Clear(model.TasksCollection)
	util.HandleTestingErr(err, t, "Error removing %v collection ", model.TasksCollection)
	err = db_util.Clear(model.HostsCollection)
	util.HandleTestingErr(err, t, "Error removing %v collection ", model.HostsCollection)
	err = db_util.Clear(model.BuildsCollection)
	util.HandleTestingErr(err, t, "Error removing %v collection ", model.BuildsCollection)
	err = db_util.Clear(model.OldTasksCollection)
	util.HandleTestingErr(err, t, "Error removing %v collection ", model.OldTasksCollection)
}

func getInterestingHosts(t *testing.T) (time.Time, []model.Host) {
	// our defacto current time
	parseLayout := "2006-01-02 15:04:05"
	_currentTime := "2013-05-07 14:17:05"
	currentTime, err := time.Parse(parseLayout, _currentTime)
	util.HandleTestingErr(err, t, "error parsing lastTaskCompletedTime")

	hosts := make([]model.Host, 0)
	host := &model.Host{}

	// create four host documents

	// host 1 should not be reaped
	// its next payment time is "2013-05-07 15:16:35" - which is in almost an hour
	// even though its last task was completed over 2 hours ago
	// host 1 should be terminated - has not been provisioned for over 20m
	// host 1 should be terminated - has been productive for hours
	// host 1 is not running a hung task
	_creationTime := "2013-05-07 10:16:35"
	_lastTaskCompletedTime := "2013-05-07 11:44:22"

	creationTime, err := time.Parse(parseLayout, _creationTime)
	util.HandleTestingErr(err, t, "error parsing creationTime")
	lastTaskCompletedTime, err := time.Parse(parseLayout, _lastTaskCompletedTime)
	util.HandleTestingErr(err, t, "error parsing lastTaskCompletedTime")
	host.Id = "noreap-1"
	host.CreationTime = creationTime
	host.LastTaskCompletedTime = lastTaskCompletedTime
	host.LastTaskCompleted = ""
	host.Provisioned = false
	host.Distro = "linux"
	host.TaskDispatchTime = creationTime.Add(time.Duration(-10 * time.Hour))
	host.RunningTask = ""
	host.Status = "running"
	host.StartedBy = mci.MCIUser
	hosts = append(hosts, *host)

	// host 2 should be reaped
	// its next payment time is "2013-05-07 14:19:35" - which is in two minutes
	// and its last task was completed 36m43s ago
	// host 2 should be terminated - has not been provisioned for over 20m
	// host 2 should not be terminated - has been productive
	// host 2 is not running a hung task
	_creationTime = "2013-03-07 10:19:35"
	_lastTaskCompletedTime = "2013-05-07 13:40:22"

	creationTime, err = time.Parse(parseLayout, _creationTime)
	util.HandleTestingErr(err, t, "error parsing creationTime")
	lastTaskCompletedTime, err = time.Parse(parseLayout, _lastTaskCompletedTime)
	util.HandleTestingErr(err, t, "error parsing lastTaskCompletedTime")
	host.Id = "reap-1"
	host.CreationTime = creationTime
	host.LastTaskCompletedTime = lastTaskCompletedTime
	host.LastTaskCompleted = "did something"
	host.Provisioned = false
	host.Distro = "linux"
	host.TaskDispatchTime = creationTime.Add(time.Duration(-11 * time.Hour))
	host.RunningTask = ""
	host.Status = "running"
	host.StartedBy = mci.MCIUser
	hosts = append(hosts, *host)

	// host 3 should not be reaped
	// while its next payment time is "2013-05-07 14:19:35"
	// its last task was completed just 10m ago
	// host 3 should be terminated - has not been provisioned for over 20m
	// host 3 not should be terminated - has not been productive for days
	// host 3 is not running a hung task
	_creationTime = "2013-01-07 08:33:35"
	_lastTaskCompletedTime = "2013-05-07 14:07:05"

	creationTime, err = time.Parse(parseLayout, _creationTime)
	util.HandleTestingErr(err, t, "error parsing creationTime")
	lastTaskCompletedTime, err = time.Parse(parseLayout, _lastTaskCompletedTime)
	util.HandleTestingErr(err, t, "error parsing lastTaskCompletedTime")
	host.Id = "noreap-2"
	host.CreationTime = creationTime
	host.LastTaskCompletedTime = lastTaskCompletedTime
	host.LastTaskCompleted = ""
	host.Provisioned = false
	host.Distro = "linux"
	host.TaskDispatchTime = creationTime.Add(time.Duration(-12 * time.Hour))
	host.RunningTask = ""
	host.Status = "decommissioned"
	host.StartedBy = mci.MCIUser
	hosts = append(hosts, *host)

	// host 4 should not be reaped
	// its next payment time is "2013-05-17 14:59:35"
	// - which is less than MinTimeToNextPayment (10m)
	// even though its last task was completed 1h10m ago
	// host 4 should not be terminated since it has 5m to go still
	// host 4 should not be terminated - while it hasn't completed
	// host 4 is also running a hung task
	// any task, it still has about 35m to be unproductive
	_creationTime = "2013-05-07 14:32:35"
	_lastTaskCompletedTime = "2013-05-07 13:07:05"

	creationTime, err = time.Parse(parseLayout, _creationTime)
	util.HandleTestingErr(err, t, "error parsing creationTime")
	lastTaskCompletedTime, err = time.Parse(parseLayout, _lastTaskCompletedTime)
	util.HandleTestingErr(err, t, "error parsing lastTaskCompletedTime")
	host.Id = "noreap-3"
	host.CreationTime = creationTime
	host.LastTaskCompletedTime = lastTaskCompletedTime
	host.LastTaskCompleted = ""
	host.Provisioned = false
	host.Distro = "windows"
	host.TaskDispatchTime = creationTime.Add(time.Duration(-13 * time.Hour))
	host.RunningTask = "some task"
	host.Status = "decommissioned"
	host.StartedBy = mci.MCIUser
	hosts = append(hosts, *host)

	return currentTime, hosts
}
