package scheduler

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
	"time"
)

var (
	taskImportanceCmpTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(
		taskImportanceCmpTestConf))
	if taskImportanceCmpTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(taskImportanceCmpTestConf.Scheduler.LogFile)
	}
}

func TestTaskImportanceComparators(t *testing.T) {

	var taskPrioritizer *CmpBasedTaskPrioritizer
	var taskIds []string
	var tasks []model.Task

	Convey("When using the task importance comparators", t, func() {

		taskPrioritizer = &CmpBasedTaskPrioritizer{}

		taskIds = []string{"t1", "t2"}

		tasks = []model.Task{
			model.Task{Id: taskIds[0]},
			model.Task{Id: taskIds[1]},
		}

		Convey("the explicit priority comparator should prioritize a task"+
			" if its priority is higher", func() {

			tasks[0].Priority = 2
			tasks[1].Priority = 2

			cmpResult, err := byPriority(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].Priority = 2
			tasks[1].Priority = 1
			cmpResult, err = byPriority(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			tasks[0].Priority = 1
			tasks[1].Priority = 2
			cmpResult, err = byPriority(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)
		})

		Convey("the stage name comparator should prioritize the appropriate"+
			" specific stage", func() {

			prioritizeCompile := byStageName(evergreen.CompileStage)
			cmpResult, err := prioritizeCompile(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].DisplayName = evergreen.CompileStage
			cmpResult, err = prioritizeCompile(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = prioritizeCompile(tasks[1], tasks[0],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[1].DisplayName = evergreen.CompileStage
			cmpResult, err = prioritizeCompile(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the dependent count comparator should prioritize a task"+
			" if its number of dependents is higher", func() {

			cmpResult, err := byNumDeps(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].NumDependents = 1
			cmpResult, err = byNumDeps(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byNumDeps(tasks[1], tasks[0],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[1].NumDependents = 1
			cmpResult, err = byNumDeps(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the commit order number comparator should prioritize a task"+
			" whose commit order number is higher, providing the tasks are"+
			" part of the same project", func() {

			cmpResult, err := byRevisionOrderNumber(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].RevisionOrderNumber = 1
			cmpResult, err = byRevisionOrderNumber(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byRevisionOrderNumber(tasks[1], tasks[0],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[0].Project = "project"
			cmpResult, err = byRevisionOrderNumber(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the create time comparator should prioritize a task whose"+
			" create time is higher, providing the tasks are from different"+
			" projects", func() {

			cmpResult, err := byCreateTime(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			// change one create time - should still be zero since the
			// projects are the same
			tasks[0].CreateTime = time.Now()
			cmpResult, err = byCreateTime(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			tasks[0].Project = "project"
			cmpResult, err = byCreateTime(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byCreateTime(tasks[1], tasks[0], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			tasks[1].CreateTime = tasks[0].CreateTime
			cmpResult, err = byCreateTime(tasks[0], tasks[1], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			cmpResult, err = byCreateTime(tasks[1], tasks[0], taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the recent failure comparator should prioritize a task"+
			" whose last execution failed", func() {

			prevTaskIds := []string{"pt1", "pt2"}

			prevTasks := map[string]model.Task{
				taskIds[0]: model.Task{Id: prevTaskIds[0]},
				taskIds[1]: model.Task{Id: prevTaskIds[1]},
			}

			taskPrioritizer.previousTasksCache = prevTasks

			cmpResult, err := byRecentlyFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			pt1 := taskPrioritizer.previousTasksCache[taskIds[0]]
			pt1.Status = evergreen.TaskFailed
			taskPrioritizer.previousTasksCache[taskIds[0]] = pt1

			cmpResult, err = byRecentlyFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			cmpResult, err = byRecentlyFailing(tasks[1], tasks[0],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			pt2 := taskPrioritizer.previousTasksCache[taskIds[1]]
			pt2.Status = evergreen.TaskFailed
			taskPrioritizer.previousTasksCache[taskIds[1]] = pt2

			cmpResult, err = byRecentlyFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})

		Convey("the similar failing comparator should prioritize a task if "+
			"it has more similar failing tasks", func() {
			prlTaskIds := []string{"t1", "t2"}

			similarFailingCountMap := map[string]int{
				prlTaskIds[0]: 3,
				prlTaskIds[1]: 3,
			}

			taskPrioritizer.similarFailingCount = similarFailingCountMap

			cmpResult, err := bySimilarFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

			taskPrioritizer.similarFailingCount[prlTaskIds[0]] = 4

			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 1)

			taskPrioritizer.similarFailingCount[prlTaskIds[1]] = 5
			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, -1)

			taskPrioritizer.similarFailingCount[prlTaskIds[0]] = 5

			cmpResult, err = bySimilarFailing(tasks[0], tasks[1],
				taskPrioritizer)
			So(err, ShouldBeNil)
			So(cmpResult, ShouldEqual, 0)

		})
	})

}
