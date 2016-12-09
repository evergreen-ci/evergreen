package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestHostTaskAuditing(t *testing.T) {
	Convey("With pre-made sets of mappings", t, func() {
		Convey("a valid mapping should return no inconsistencies", func() {
			h2t := map[string]string{"h1": "t1", "h2": "t2", "h3": "t3"}
			t2h := map[string]string{"t1": "h1", "t2": "h2", "t3": "h3"}
			So(len(auditHostTaskMapping(h2t, t2h)), ShouldEqual, 0)
		})
		Convey("a mismapped host should return one inconsistency", func() {
			h2t := map[string]string{"h1": "t1", "h2": "t2", "h3": "t3", "h4": "t1"}
			t2h := map[string]string{"t1": "h4", "t2": "h2", "t3": "h3"}
			out := auditHostTaskMapping(h2t, t2h)
			So(len(out), ShouldEqual, 1)
			So(out[0], ShouldResemble, HostTaskInconsistency{
				Host: "h1", HostTaskCache: "t1", Task: "t1", TaskHostCache: "h4",
			})
		})
		Convey("a swapped host and task should return four inconsistencies", func() {
			h2t := map[string]string{"h1": "t3", "h2": "t2", "h3": "t1"}
			t2h := map[string]string{"t1": "h1", "t2": "h2", "t3": "h3"}
			out := auditHostTaskMapping(h2t, t2h)
			So(len(out), ShouldEqual, 4)
		})
		Convey("one empty mapping should return inconsistencies", func() {
			h2t := map[string]string{"h1": "t1", "h2": "t2", "h3": "t3"}
			out := auditHostTaskMapping(h2t, nil)
			So(len(out), ShouldEqual, 3)
			Convey("with reasonable error language", func() {
				So(out[0].Error(), ShouldContainSubstring, "does not exist")
				So(out[1].Error(), ShouldContainSubstring, "does not exist")
				So(out[2].Error(), ShouldContainSubstring, "does not exist")
			})
		})
		Convey("two empty mappings should not return inconsistencies", func() {
			out := auditHostTaskMapping(nil, nil)
			So(len(out), ShouldEqual, 0)
		})
	})

	Convey("With tasks and hosts stored in the db", t, func() {
		testutil.HandleTestingErr(db.Clear(host.Collection), t,
			"Error clearing '%v' collection", host.Collection)
		testutil.HandleTestingErr(db.Clear(task.Collection), t,
			"Error clearing '%v' collection", task.Collection)
		Convey("no mappings should load with an empty db", func() {
			h2t, t2h, err := loadHostTaskMapping()
			So(err, ShouldBeNil)
			So(len(h2t), ShouldEqual, 0)
			So(len(t2h), ShouldEqual, 0)
		})
		Convey("with 3 hosts, one with a running task", func() {
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning, RunningTask: "t1"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning}
			h3 := host.Host{Id: "h3", Status: evergreen.HostRunning}
			So(h1.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)
			So(h3.Insert(), ShouldBeNil)
			Convey("only mappings should return for the task-running host", func() {
				h2t, t2h, err := loadHostTaskMapping()
				So(err, ShouldBeNil)
				So(len(h2t), ShouldEqual, 1)
				So(h2t["h1"], ShouldEqual, "t1")
				So(len(t2h), ShouldEqual, 0)
			})
		})
		Convey("with 3 hosts and 3 tasks", func() {
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning, RunningTask: "t1"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, RunningTask: "t2"}
			h3 := host.Host{Id: "h3", Status: evergreen.HostRunning}
			t1 := task.Task{Id: "t1", Status: evergreen.TaskStarted, HostId: "h1"}
			t2 := task.Task{Id: "t2", Status: evergreen.TaskDispatched, HostId: "h2"}
			t3 := task.Task{Id: "t3"}
			So(h1.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)
			So(h3.Insert(), ShouldBeNil)
			So(t1.Insert(), ShouldBeNil)
			So(t2.Insert(), ShouldBeNil)
			So(t3.Insert(), ShouldBeNil)
			Convey("mappings should return for the task-running hosts and running tasks", func() {
				h2t, t2h, err := loadHostTaskMapping()
				So(err, ShouldBeNil)
				So(len(h2t), ShouldEqual, 2)
				So(h2t["h1"], ShouldEqual, "t1")
				So(h2t["h2"], ShouldEqual, "t2")
				So(len(t2h), ShouldEqual, 2)
				So(t2h["t1"], ShouldEqual, "h1")
				So(t2h["t2"], ShouldEqual, "h2")
			})
		})

	})
}
