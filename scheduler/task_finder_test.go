package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	taskFinderTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskFinderTestConf))
	if taskFinderTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(taskFinderTestConf.Scheduler.LogFile)
	}
}

func TestDBTaskFinder(t *testing.T) {

	var taskIds []string
	var tasks []*task.Task
	var depTaskIds []string
	var depTasks []*task.Task
	var taskFinder *DBTaskFinder

	Convey("With a DBTaskFinder", t, func() {

		taskFinder = &DBTaskFinder{}

		taskIds = []string{"t1", "t2", "t3", "t4"}
		tasks = []*task.Task{
			&task.Task{Id: taskIds[0], Status: evergreen.TaskUndispatched, Activated: true},
			&task.Task{Id: taskIds[1], Status: evergreen.TaskUndispatched, Activated: true},
			&task.Task{Id: taskIds[2], Status: evergreen.TaskUndispatched, Activated: true},
			&task.Task{Id: taskIds[3], Status: evergreen.TaskUndispatched, Activated: true, Priority: -1},
		}

		depTaskIds = []string{"td1", "td2"}
		depTasks = []*task.Task{
			&task.Task{Id: depTaskIds[0]},
			&task.Task{Id: depTaskIds[1]},
		}

		So(db.Clear(task.Collection), ShouldBeNil)

		Convey("if there are no runnable tasks, an empty slice (with no error)"+
			" should be returned", func() {
			runnableTasks, err := taskFinder.FindRunnableTasks()
			So(err, ShouldBeNil)
			So(len(runnableTasks), ShouldEqual, 0)
		})

		Convey("inactive tasks should not be returned", func() {

			// insert the tasks, setting one to inactive
			tasks[2].Activated = false
			for _, testTask := range tasks {
				So(testTask.Insert(), ShouldBeNil)
			}

			// finding the runnable tasks should return two tasks
			runnableTasks, err := taskFinder.FindRunnableTasks()
			So(err, ShouldBeNil)
			So(len(runnableTasks), ShouldEqual, 2)

		})

		Convey("tasks with unmet dependencies should not be returned", func() {

			// insert the dependency tasks, setting one to have finished
			// successfully and one to have finished unsuccessfully
			depTasks[0].Status = evergreen.TaskFailed
			depTasks[1].Status = evergreen.TaskSucceeded
			for _, depTask := range depTasks {
				So(depTask.Insert(), ShouldBeNil)
			}

			// insert the tasks, setting one to have unmet dependencies, one to
			// have no dependencies, and one to have successfully met
			// dependencies
			tasks[0].DependsOn = []task.Dependency{}
			tasks[1].DependsOn = []task.Dependency{{depTasks[0].Id, evergreen.TaskSucceeded}}
			tasks[2].DependsOn = []task.Dependency{{depTasks[1].Id, evergreen.TaskSucceeded}}
			for _, testTask := range tasks {
				So(testTask.Insert(), ShouldBeNil)
			}

			// finding the runnable tasks should return two tasks (the one with
			// no dependencies and the one with successfully met dependencies
			runnableTasks, err := taskFinder.FindRunnableTasks()
			So(err, ShouldBeNil)
			So(len(runnableTasks), ShouldEqual, 2)

		})

	})

}
