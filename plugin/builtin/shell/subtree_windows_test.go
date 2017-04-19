// +build windows

package shell

import (
	"fmt"
	"sync"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	err := grip.SetSender(send.MakeNative())
	grip.CatchError(err)
}

func TestWindowsProcessRegistry(t *testing.T) {
	reg := newProcessRegistry()
	Convey("the process registry", t, func() {
		So(len(reg.jobs), ShouldEqual, 0)

		Reset(func() {
			reg = newProcessRegistry()
			So(len(reg.jobs), ShouldEqual, 0)
		})

		Convey("should store new jobs", func() {
			for i := 0; i < 10; i++ {
				id := fmt.Sprintf("job-%d", i)
				j, err := reg.getJob(id)
				So(j, ShouldNotBeNil)
				So(err, ShouldBeNil)
			}
			So(len(reg.jobs), ShouldEqual, 10)
		})

		Convey("should support concurrent operations", func(c C) {
			const (
				numWorkers = 10
				numJobs    = 40
			)

			wg := &sync.WaitGroup{}

			for w := 0; w < numWorkers; w++ {
				wg.Add(1)
				go func(num int, c C) {
					defer wg.Done()
					for i := 0; i < numJobs; i++ {
						id := fmt.Sprintf("job-%d.%d", num, i)
						j, err := reg.getJob(id)
						c.So(j, ShouldNotBeNil)
						c.So(err, ShouldBeNil)
					}
				}(w, c)
			}

			wg.Wait()
			So(len(reg.jobs), ShouldEqual, numJobs*numWorkers)
		})

		Convey("should de-duplicate jobs by given id", func() {
			ids := []string{"one", "two", "three"}
			for i := 0; i < 10; i++ {
				for _, id := range ids {
					j, err := reg.getJob(id)
					So(j, ShouldNotBeNil)
					So(err, ShouldBeNil)
				}
			}

			So(len(reg.jobs), ShouldEqual, len(ids))
		})

		Convey("remove should try to remove tracked jobs", func() {
			const id = "jobName"
			j, err := reg.getJob(id)
			So(j, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(len(reg.jobs), ShouldEqual, 1)

			grip.CatchError(reg.removeJob(id))
			So(len(reg.jobs), ShouldEqual, 0)
		})
	})
}
