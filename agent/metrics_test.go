package agent

import (
	"os/exec"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/comm"
	"github.com/evergreen-ci/evergreen/model"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/tychoish/grip/message"
)

func newTestCommunicator() *comm.MockCommunicator {
	return &comm.MockCommunicator{
		LogChan: make(chan []model.LogMessage, 100),
		Posts:   map[string][]interface{}{},
	}
}

func TestMetricsCollectors(t *testing.T) {
	Convey("The metrics collector,", t, func() {
		Convey("should persist process stats", func() {
			Convey("process collector runs for interval and sends messages", func() {
				stopper := make(chan struct{})
				comm := newTestCommunicator()
				collector := metricsCollector{
					comm: comm,
					stop: stopper,
				}

				So(len(comm.Posts["process_info"]), ShouldEqual, 0)

				go collector.processInfoCollector(500*time.Millisecond, time.Second, 2)
				time.Sleep(time.Second)
				stopper <- struct{}{}
				So(len(comm.Posts["process_info"]), ShouldEqual, 2)
				// after stopping it shouldn't continue to collect stats
				time.Sleep(time.Second)
				So(len(comm.Posts["process_info"]), ShouldEqual, 2)

				for _, post := range comm.Posts["process_info"] {
					out, ok := post.([]message.Composer)
					So(ok, ShouldBeTrue)
					So(len(out), ShouldEqual, 1)
				}

			})

			Convey("process collector should collect sub-processes", func() {
				stopper := make(chan struct{})
				comm := newTestCommunicator()
				collector := metricsCollector{
					comm: comm,
					stop: stopper,
				}

				So(len(comm.Posts["process_info"]), ShouldEqual, 0)

				cmd := exec.Command("bash", "-c", "'start'; sleep 100; echo 'finish'")
				So(cmd.Start(), ShouldBeNil)
				go collector.processInfoCollector(500*time.Millisecond, time.Second, 2)
				time.Sleep(time.Second)
				stopper <- struct{}{}
				cmd.Process.Kill()

				So(len(comm.Posts["process_info"]), ShouldEqual, 2)
				for _, post := range comm.Posts["process_info"] {
					out, ok := post.([]message.Composer)
					So(ok, ShouldBeTrue)
					So(len(out), ShouldEqual, 2)
				}
			})
		})
		Convey("should persist system stats", func() {
			stopper := make(chan struct{})
			comm := newTestCommunicator()
			collector := metricsCollector{
				comm: comm,
				stop: stopper,
			}

			So(len(comm.Posts["system_info"]), ShouldEqual, 0)
			go collector.sysInfoCollector(500 * time.Millisecond)
			time.Sleep(time.Second)
			stopper <- struct{}{}

			So(len(comm.Posts["system_info"]), ShouldEqual, 2)
			time.Sleep(time.Second)
			So(len(comm.Posts["system_info"]), ShouldEqual, 2)
		})
	})
}
