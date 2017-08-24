package agent

import (
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip/message"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func newTestCommunicator() *comm.MockCommunicator {
	return &comm.MockCommunicator{
		LogChan: make(chan []apimodels.LogMessage, 100),
		Posts:   map[string][]interface{}{},
	}
}

func TestMetricsCollectors(t *testing.T) {
	Convey("The metrics collector should", t, func() {
		Convey("process collector runs for interval and sends messages", func() {
			comm := newTestCommunicator()
			ctx, cancel := context.WithCancel(context.Background())
			collector := metricsCollector{
				comm: comm,
			}

			So(len(comm.Posts["process_info"]), ShouldEqual, 0)

			go collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
			time.Sleep(time.Second)
			firstLen := len(comm.Posts["process_info"])
			if runtime.GOOS == "windows" {
				So(firstLen, ShouldBeGreaterThanOrEqualTo, 1)
			} else {
				So(firstLen, ShouldBeGreaterThanOrEqualTo, 2)
			}
			cancel()
			// after stopping it shouldn't continue to collect stats
			time.Sleep(time.Second)
			So(firstLen, ShouldEqual, len(comm.Posts["process_info"]))

			for _, post := range comm.Posts["process_info"] {
				out, ok := post.([]message.Composer)
				So(ok, ShouldBeTrue)
				So(len(out), ShouldEqual, 1)
			}

		})

		Convey("process collector should collect sub-processes", func() {
			ctx, cancel := context.WithCancel(context.Background())
			comm := newTestCommunicator()
			collector := metricsCollector{
				comm: comm,
			}

			So(len(comm.Posts["process_info"]), ShouldEqual, 0)

			cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
			So(cmd.Start(), ShouldBeNil)
			go collector.processInfoCollector(ctx, 750*time.Millisecond, time.Second, 2)
			time.Sleep(time.Second)
			So(cmd.Process.Kill(), ShouldBeNil)
			cancel()

			if runtime.GOOS == "windows" {
				So(len(comm.Posts["process_info"]), ShouldEqual, 1)
			} else {
				So(len(comm.Posts["process_info"]), ShouldEqual, 2)
			}
			for _, post := range comm.Posts["process_info"] {
				out, ok := post.([]message.Composer)
				So(ok, ShouldBeTrue)

				// the number of posts is different on windows,
				if runtime.GOOS == "windows" {
					So(len(out), ShouldBeGreaterThanOrEqualTo, 1)
				} else {
					So(len(out), ShouldBeGreaterThanOrEqualTo, 2)
				}

			}
		})

		Convey("persist system stats", func() {
			ctx, cancel := context.WithCancel(context.Background())
			comm := newTestCommunicator()
			collector := metricsCollector{
				comm: comm,
			}

			So(len(comm.Posts["system_info"]), ShouldEqual, 0)
			go collector.sysInfoCollector(ctx, 750*time.Millisecond)
			time.Sleep(time.Second)
			So(len(comm.Posts["system_info"]), ShouldBeGreaterThanOrEqualTo, 1)
			time.Sleep(time.Second)
			So(len(comm.Posts["system_info"]), ShouldBeGreaterThanOrEqualTo, 1)
			cancel()
		})
	})
}
