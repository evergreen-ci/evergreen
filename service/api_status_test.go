package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

// getCTAEndpoint is a helper that creates a test API server,
// GETs the consistent_task_assignment endpoint, and returns
// the response.
func getCTAEndpoint(t *testing.T) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	as, err := NewAPIServer(testutil.TestConfig(), nil)
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
	return w
}

// EVG-1602: this fixture is not used yet. Commented to avoid deadcode lint error.
func getStuckHostEndpoint(t *testing.T) *httptest.ResponseRecorder {
	if err := os.MkdirAll(filepath.Join(evergreen.FindEvergreenHome(), evergreen.ClientDirectory), 0644); err != nil {
		t.Fatal("could not create client directory required to start the API server:", err.Error())
	}

	as, err := NewAPIServer(testutil.TestConfig(), nil)
	if err != nil {
		t.Fatalf("creating test API server: %v", err)
	}
	handler, err := as.Handler()
	if err != nil {
		t.Fatalf("creating test API handler: %v", err)
	}
	url := "/api/status/stuck_hosts"
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("building request: %v", err)
	}

	w := httptest.NewRecorder()
	handler.ServeHTTP(w, request)
	return w
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
				So(resp.Code, ShouldEqual, http.StatusOK)
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
				So(resp.Code, ShouldEqual, http.StatusOK)
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
					So(len(tar.TaskIds), ShouldEqual, 1)
					So(len(tar.HostRunningTasks), ShouldEqual, 2)
					So(tar.TaskIds, ShouldContain, "t2")
					So(tar.HostRunningTasks, ShouldContain, "t1000")
				})
			})
		})
	})
}

func TestServiceStatusEndPoints(t *testing.T) {
	testConfig := testutil.TestConfig()
	testServer, err := CreateTestServer(testConfig, nil, plugin.APIPlugins)
	testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
	defer testServer.Close()

	url := fmt.Sprintf("%s/api/status/info", testServer.URL)

	Convey("Service Status endpoints should report the status of the service", t, func() {
		Convey("basic endpoint should have one key, that reports the build id", func() {
			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)

			resp, err := http.DefaultClient.Do(request)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
			out := map[string]string{}

			So(util.ReadJSONInto(resp.Body, &out), ShouldBeNil)
			So(len(out), ShouldEqual, 1)
			_, ok := out["build_revision"]
			So(ok, ShouldBeTrue)
		})
		Convey("auth endpoint should report extended information", func() {
			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)
			request.AddCookie(&http.Cookie{Name: evergreen.AuthTokenCookie, Value: "token"})

			resp, err := http.DefaultClient.Do(request)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
			out := map[string]interface{}{}

			So(util.ReadJSONInto(resp.Body, &out), ShouldBeNil)

			So(len(out), ShouldEqual, 3)
			for _, key := range []string{"build_revision", "sys_info", "pid"} {
				_, ok := out[key]
				So(ok, ShouldBeTrue)
			}

		})
	})
}

func TestStuckHostEndpoints(t *testing.T) {
	Convey("With a test server and test config", t, func() {
		testConfig := testutil.TestConfig()
		testServer, err := CreateTestServer(testConfig, nil, plugin.APIPlugins)
		testutil.HandleTestingErr(err, t, "Couldn't create apiserver: %v", err)
		defer testServer.Close()

		if err := db.ClearCollections(host.Collection, task.Collection); err != nil {
			t.Fatalf("clearing db: %v", err)
		}

		url := fmt.Sprintf("%s/api/status/stuck_hosts", testServer.URL)
		Convey("With hosts and tasks that are all consistent, the response should success", func() {
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

			request, err := http.NewRequest("GET", url, nil)
			So(err, ShouldBeNil)

			resp, err := http.DefaultClient.Do(request)
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
			out := stuckHostResp{}

			So(util.ReadJSONInto(resp.Body, &out), ShouldBeNil)
			So(out.Status, ShouldEqual, apiStatusSuccess)

		})
		Convey("With hosts that have running tasks that have completed", func() {
			h1 := host.Host{Id: "h1", Status: evergreen.HostRunning, RunningTask: "t1"}
			h2 := host.Host{Id: "h2", Status: evergreen.HostRunning, RunningTask: "t2"}
			h3 := host.Host{Id: "h3", Status: evergreen.HostRunning}
			t1 := task.Task{Id: "t1", Status: evergreen.TaskStarted, HostId: "h1"}
			t2 := task.Task{Id: "t2", Status: evergreen.TaskFailed, HostId: "h2"}
			t3 := task.Task{Id: "t3", Status: evergreen.TaskFailed}

			So(h1.Insert(), ShouldBeNil)
			So(h2.Insert(), ShouldBeNil)
			So(h3.Insert(), ShouldBeNil)
			So(t1.Insert(), ShouldBeNil)
			So(t2.Insert(), ShouldBeNil)
			So(t3.Insert(), ShouldBeNil)

			resp := getStuckHostEndpoint(t)
			So(resp, ShouldNotBeNil)
			So(resp.Code, ShouldEqual, http.StatusOK)

			out := stuckHostResp{}

			So(json.NewDecoder(resp.Body).Decode(&out), ShouldBeNil)
			So(out.Status, ShouldEqual, apiStatusError)
			So(len(out.HostIds), ShouldEqual, 1)
			So(len(out.TaskIds), ShouldEqual, 1)
			So(out.HostIds[0], ShouldEqual, "h2")
			So(out.TaskIds[0], ShouldEqual, "t2")

		})
	})
}
