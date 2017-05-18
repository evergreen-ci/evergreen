package agent

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
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
			grip.Alert(strings.Join(resp.SystemInfo.Errors, ";\n"))
			grip.Info(resp.SystemInfo)
			So(resp.SystemInfo, ShouldNotBeNil)
		})

		Convey("the process tree information should be populated", func() {
			So(len(resp.ProcessTree), ShouldBeGreaterThanOrEqualTo, 1)
			for _, ps := range resp.ProcessTree {
				So(ps, ShouldNotBeNil)
			}
		})

		Convey("the response should include task_id", func() {
			So(resp.TaskId, ShouldEqual, testTaskIdValue)
		})
	})
}

func TestAgentConstructorStartsStatusServer(t *testing.T) {
	testOpts := Options{
		APIURL:     "http://evergreen.example.net",
		HostId:     "none",
		StatusPort: 2286,
	}

	Convey("the agent constructor", t, func() {
		agt, err := New(testOpts)
		So(err, ShouldBeNil)
		defer agt.stop()

		time.Sleep(100 * time.Millisecond)

		Convey("should start a status server.", func() {
			resp, err := http.Get("http://127.0.0.1:2286/status")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
		})
	})
}
