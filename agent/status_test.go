package agent

import (
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestAgentStatusHandler(t *testing.T) {
	testOpts := &Options{
		APIURL: "http://evergreen.example.net",
		HostId: "none",
	}

	Convey("The agent status results producer should reflect the current app", t, func() {
		resp := buildResponse(testOpts)
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
	})
}
