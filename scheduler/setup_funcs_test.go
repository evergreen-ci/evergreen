package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestSetupFuncs(t *testing.T) {

	var taskComparator *CmpBasedTaskComparator
	var taskIds []string
	var tasks []task.Task

	Convey("When running the setup funcs for task prioritizing", t, func() {

		taskComparator = &CmpBasedTaskComparator{}

		taskIds = []string{"t1", "t2", "t3"}

		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
			{Id: taskIds[2]},
		}

		testutil.HandleTestingErr(
			db.ClearCollections(build.Collection, task.Collection),
			t, "Failed to clear test collections")

		Convey("the previous task caching setup func should fetch and save the"+
			" relevant previous runs of tasks", func() {

			displayNames := []string{"disp1", "disp2", "disp3"}
			buildVariant := "bv"
			prevTaskIds := []string{"pt1", "pt2", "pt3"}
			project := "project"

			tasks[0].RevisionOrderNumber = 100
			tasks[0].Requester = evergreen.RepotrackerVersionRequester
			tasks[0].DisplayName = displayNames[0]
			tasks[0].BuildVariant = buildVariant
			tasks[0].Project = project

			tasks[1].RevisionOrderNumber = 200
			tasks[1].Requester = evergreen.RepotrackerVersionRequester
			tasks[1].DisplayName = displayNames[1]
			tasks[1].BuildVariant = buildVariant
			tasks[1].Project = project

			tasks[2].RevisionOrderNumber = 300
			tasks[2].Requester = evergreen.RepotrackerVersionRequester
			tasks[2].DisplayName = displayNames[2]
			tasks[2].BuildVariant = buildVariant
			tasks[2].Project = project

			// the previous tasks

			prevTaskOne := &task.Task{
				Id:                  prevTaskIds[0],
				RevisionOrderNumber: 99,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[0],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskFailed,
			}

			prevTaskTwo := &task.Task{
				Id:                  prevTaskIds[1],
				RevisionOrderNumber: 199,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[1],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskSucceeded,
			}

			prevTaskThree := &task.Task{
				Id:                  prevTaskIds[2],
				RevisionOrderNumber: 299,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[2],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskSucceeded,
			}

			So(prevTaskOne.Insert(), ShouldBeNil)
			So(prevTaskTwo.Insert(), ShouldBeNil)
			So(prevTaskThree.Insert(), ShouldBeNil)

			taskComparator.tasks = tasks
			So(cachePreviousTasks(taskComparator), ShouldBeNil)
			So(len(taskComparator.previousTasksCache), ShouldEqual, 3)
			So(taskComparator.previousTasksCache[taskIds[0]].Id, ShouldEqual,
				prevTaskIds[0])
			So(taskComparator.previousTasksCache[taskIds[1]].Id, ShouldEqual,
				prevTaskIds[1])
			So(taskComparator.previousTasksCache[taskIds[2]].Id, ShouldEqual,
				prevTaskIds[2])

		})
	})
}
