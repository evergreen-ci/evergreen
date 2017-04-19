package agent

import (
	"net/http"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAgentStatusHandler(t *testing.T) {
	testOpts := Options{
		APIURL: "http://evergreen.example.net",
		HostId: "none",
	}
	testTaskIdValue := "test_task_id"

	Convey("The agent status results producer should reflect the current app", t, func() {
		resp := buildResponse(testOpts, testTaskIdValue)
		Convey("the status document should reflect basic assumptions", func() {
			So(resp.BuildId, ShouldEqual, evergreen.BuildRevision)
			So(resp.AgentPid, ShouldEqual, os.Getpid())
			So(resp.HostId, ShouldEqual, testOpts.HostId)
		})

		Convey("the system information should be populated", func() {
			So(resp.SystemInfo, ShouldNotBeNil)
			So(len(resp.SystemInfo.Errors), ShouldEqual, 0)
		})

		Convey("the process tree information should be populated", func() {
			So(len(resp.ProcessTree), ShouldBeGreaterThanOrEqualTo, 1)
			if len(resp.ProcessTree) == 1 {
				So(len(resp.ProcessTree[0].Errors), ShouldEqual, 1)
			} else {
				for _, ps := range resp.ProcessTree {
					So(ps, ShouldNotBeNil)
				}
			}
		})

		Convey("the response should include task_id", func() {
			So(resp.TaskId, ShouldEqual, testTaskIdValue)
		})
	})
}

func TestAgentConstructorStartsServer(t *testing.T) {
	testOpts := Options{
		APIURL:     "http://evergreen.example.net",
		HostId:     "none",
		StatusPort: 2285,
	}

	Convey("the agent constructor", t, func() {
		agt, err := New(testOpts)
		So(err, ShouldBeNil)
		defer agt.stop()

		Convey("should start a status server.", func() {
			resp, err := http.Get("http://127.0.0.1:2285/status")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
		})
	})
}
