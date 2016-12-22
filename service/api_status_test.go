package service

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"

	"net/http"
	"net/http/httptest"
	"testing"
)

// getCTAEndpoint is a helper that creates a test API server,
// GETs the consistent_task_assignment endpoint, and returns
// the response.
func getCTAEndpoint(t *testing.T) *http.Response {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	as, err := NewAPIServer(evergreen.TestConfig(), nil)
	if err != nil {
		t.Fatalf("creating test API server: %v", err)
	}
	handler, err := as.Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := "/api/status/consistent_task_assignment"
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w.Result()
}

func TestConsistentTaskAssignment(t *testing.T) {

	Convey("With various states of tasks and hosts in the DB", t, func() {
		if err := db.ClearCollections(host.Collection, task.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}
		Convey("A correct host/task mapping", func() {
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning, RunningTask: "t1"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, RunningTask: "t2"}
			h3 := host.Host{Id: "h3", Status: evergreen.HostRunning}
			t1 := task.Task{Id: "t1", Status: evergreen.TaskStarted, HostId: "h1"}
			t2 := task.Task{Id: "t2", Status: evergreen.TaskDispatched, HostId: "h2"}
			t3 := task.Task{Id: "t3", Status: evergreen.TaskFailed}
			So(h1.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)
			So(h3.Insert(), ShouldBeNil)
			So(t1.Insert(), ShouldBeNil)
			So(t2.Insert(), ShouldBeNil)
			So(t3.Insert(), ShouldBeNil)
			resp := getCTAEndpoint(t)
			So(resp, ShouldNotBeNil)
			Convey("should return HTTP 200", func() {
				So(resp.StatusCode, ShouldEqual, http.StatusOK)
				Convey("and JSON with a SUCCESS message and nothing else", func() {
					tar := taskAssignmentResp{}
					So(json.NewDecoder(resp.Body).Decode(&tar), ShouldBeNil)
					So(tar.Status, ShouldEqual, apiStatusSuccess)
					So(len(tar.Errors), ShouldEqual, 0)
					So(len(tar.HostIds), ShouldEqual, 0)
					So(len(tar.TaskIds), ShouldEqual, 0)
				})
			})
		})
		Convey("An incorrect host/task mapping", func() {
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning, RunningTask: "t1"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, RunningTask: "t2"}
			h3 := host.Host{Id: "h3", Status: evergreen.HostRunning, RunningTask: "t1000"}
			t1 := task.Task{Id: "t1", Status: evergreen.TaskStarted, HostId: "h1"}
			t2 := task.Task{Id: "t2", Status: evergreen.TaskDispatched, HostId: "h3"}
			t3 := task.Task{Id: "t3", Status: evergreen.TaskFailed}
			So(h1.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)
			So(h3.Insert(), ShouldBeNil)
			So(t1.Insert(), ShouldBeNil)
			So(t2.Insert(), ShouldBeNil)
			So(t3.Insert(), ShouldBeNil)
			resp := getCTAEndpoint(t)
			So(resp, ShouldNotBeNil)
			Convey("should return HTTP 200", func() {
				So(resp.StatusCode, ShouldEqual, http.StatusOK)
				Convey("and JSON with an ERROR message and info about each issue", func() {
					tar := taskAssignmentResp{}
					So(json.NewDecoder(resp.Body).Decode(&tar), ShouldBeNil)
					So(tar.Status, ShouldEqual, apiStatusError)
					// ERROR 1: h2 thinks it is running t2, which thinks it is running on h3
					// ERROR 2: h3 thinks it is running t1000, which does not exist
					// ERROR 3: t2 thinks it is running on h3, which thinks it is running t1000
					So(len(tar.Errors), ShouldEqual, 3)
					So(len(tar.HostIds), ShouldEqual, 2)
					So(tar.HostIds, ShouldContain, "h2")
					So(tar.HostIds, ShouldContain, "h3")
					So(len(tar.TaskIds), ShouldEqual, 2)
					So(tar.TaskIds, ShouldContain, "t2")
					So(tar.TaskIds, ShouldContain, "t1000")
				})
			})
		})
	})
}
