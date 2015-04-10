package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	setupFuncsTestConf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskFinderTestConf))
	if setupFuncsTestConf.Scheduler.LogFile != "" {
		mci.SetLogger(setupFuncsTestConf.Scheduler.LogFile)
	}
}

func TestSetupFuncs(t *testing.T) {

	var taskPrioritizer *CmpBasedTaskPrioritizer
	var taskIds []string
	var tasks []model.Task

	Convey("When running the setup funcs for task prioritizing", t, func() {

		taskPrioritizer = &CmpBasedTaskPrioritizer{}

		taskIds = []string{"t1", "t2", "t3"}

		tasks = []model.Task{
			model.Task{Id: taskIds[0]},
			model.Task{Id: taskIds[1]},
			model.Task{Id: taskIds[2]},
		}

		util.HandleTestingErr(
			db.ClearCollections(model.BuildsCollection, model.TasksCollection),
			t, "Failed to clear test collections")

		Convey("the previous task caching setup func should fetch and save the"+
			" relevant previous runs of tasks", func() {

			displayNames := []string{"disp1", "disp2", "disp3"}
			buildVariant := "bv"
			prevTaskIds := []string{"pt1", "pt2", "pt3"}
			project := "project"

			tasks[0].RevisionOrderNumber = 100
			tasks[0].Requester = mci.RepotrackerVersionRequester
			tasks[0].DisplayName = displayNames[0]
			tasks[0].BuildVariant = buildVariant
			tasks[0].Project = project

			tasks[1].RevisionOrderNumber = 200
			tasks[1].Requester = mci.RepotrackerVersionRequester
			tasks[1].DisplayName = displayNames[1]
			tasks[1].BuildVariant = buildVariant
			tasks[1].Project = project

			tasks[2].RevisionOrderNumber = 300
			tasks[2].Requester = mci.RepotrackerVersionRequester
			tasks[2].DisplayName = displayNames[2]
			tasks[2].BuildVariant = buildVariant
			tasks[2].Project = project

			// the previous tasks

			prevTaskOne := &model.Task{
				Id:                  prevTaskIds[0],
				RevisionOrderNumber: 99,
				Requester:           mci.RepotrackerVersionRequester,
				DisplayName:         displayNames[0],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              mci.TaskFailed,
			}

			prevTaskTwo := &model.Task{
				Id:                  prevTaskIds[1],
				RevisionOrderNumber: 199,
				Requester:           mci.RepotrackerVersionRequester,
				DisplayName:         displayNames[1],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              mci.TaskSucceeded,
			}

			prevTaskThree := &model.Task{
				Id:                  prevTaskIds[2],
				RevisionOrderNumber: 299,
				Requester:           mci.RepotrackerVersionRequester,
				DisplayName:         displayNames[2],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              mci.TaskSucceeded,
			}

			So(prevTaskOne.Insert(), ShouldBeNil)
			So(prevTaskTwo.Insert(), ShouldBeNil)
			So(prevTaskThree.Insert(), ShouldBeNil)

			taskPrioritizer.tasks = tasks
			So(cachePreviousTasks(taskPrioritizer), ShouldBeNil)
			So(len(taskPrioritizer.previousTasksCache), ShouldEqual, 3)
			So(taskPrioritizer.previousTasksCache[taskIds[0]].Id, ShouldEqual,
				prevTaskIds[0])
			So(taskPrioritizer.previousTasksCache[taskIds[1]].Id, ShouldEqual,
				prevTaskIds[1])
			So(taskPrioritizer.previousTasksCache[taskIds[2]].Id, ShouldEqual,
				prevTaskIds[2])

		})
	})
}
