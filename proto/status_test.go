package proto

import (
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestAgentStatusHandler(t *testing.T) {
	testOpts := Options{
		APIURL: "http://evergreen.example.net",
		HostID: "none",
	}
	Convey("The agent status results producer should reflect the current app", t, func() {
		resp := buildResponse(testOpts)
		Convey("the status document should reflect basic assumptions", func() {
			So(resp.BuildId, ShouldEqual, evergreen.BuildRevision)
			So(resp.AgentPid, ShouldEqual, os.Getpid())
			So(resp.HostId, ShouldEqual, testOpts.HostID)
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
	})
}

func TestAgentConstructorStartsStatusServer(t *testing.T) {
	testOpts := Options{
		APIURL:     "http://evergreen.example.net",
		HostID:     "none",
		StatusPort: 2286,
	}

	Convey("the agent constructor", t, func() {
		agt := New(testOpts, client.NewMock("url"))
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		agt.Start(ctx)

		Convey("should start a status server.", func() {
			resp, err := http.Get("http://127.0.0.1:2286/status")
			So(err, ShouldBeNil)
			So(resp.StatusCode, ShouldEqual, 200)
		})
	})
}
