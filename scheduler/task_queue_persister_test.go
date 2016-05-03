package scheduler

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	taskQueuePersisterTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(
		taskQueuePersisterTestConf))
	if taskQueuePersisterTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(taskQueuePersisterTestConf.Scheduler.LogFile)
	}
}

func TestDBTaskQueuePersister(t *testing.T) {

	var taskQueuePersister *DBTaskQueuePersister
	var distroIds []string
	var displayNames []string
	var buildVariants []string
	var RevisionOrderNumbers []int
	var requesters []string
	var gitspecs []string
	var projects []string
	var taskIds []string
	var durations []time.Duration
	var tasks []task.Task

	Convey("With a DBTaskQueuePersister", t, func() {

		taskQueuePersister = &DBTaskQueuePersister{}
		distroIds = []string{"d1", "d2"}
		taskIds = []string{"t1", "t2", "t3", "t4", "t5"}
		displayNames = []string{"dn1", "dn2", "dn3", "dn4", "dn5"}
		buildVariants = []string{"bv1", "bv2", "bv3", "bv4", "bv5"}
		RevisionOrderNumbers = []int{0, 1, 2, 3, 4}
		requesters = []string{"r1", "r2", "r3", "r4", "r5"}
		gitspecs = []string{"g1", "g2", "g3", "g4", "g5"}
		projects = []string{"p1", "p2", "p3", "p4", "p5"}
		durations = []time.Duration{
			time.Duration(1) * time.Minute,
			time.Duration(2) * time.Minute,
			time.Duration(3) * time.Minute,
			time.Duration(4) * time.Minute,
		}

		tasks = []task.Task{
			task.Task{
				Id:                  taskIds[0],
				DisplayName:         displayNames[0],
				BuildVariant:        buildVariants[0],
				RevisionOrderNumber: RevisionOrderNumbers[0],
				Requester:           requesters[0],
				Revision:            gitspecs[0],
				Project:             projects[0],
			},
			task.Task{
				Id:                  taskIds[1],
				DisplayName:         displayNames[1],
				BuildVariant:        buildVariants[1],
				RevisionOrderNumber: RevisionOrderNumbers[1],
				Requester:           requesters[1],
				Revision:            gitspecs[1],
				Project:             projects[1],
			},
			task.Task{
				Id:                  taskIds[2],
				DisplayName:         displayNames[2],
				BuildVariant:        buildVariants[2],
				RevisionOrderNumber: RevisionOrderNumbers[2],
				Requester:           requesters[2],
				Revision:            gitspecs[2],
				Project:             projects[2],
			},
			task.Task{
				Id:                  taskIds[3],
				DisplayName:         displayNames[3],
				BuildVariant:        buildVariants[3],
				RevisionOrderNumber: RevisionOrderNumbers[3],
				Requester:           requesters[3],
				Revision:            gitspecs[3],
				Project:             projects[3],
			},
			task.Task{
				Id:                  taskIds[4],
				DisplayName:         displayNames[4],
				BuildVariant:        buildVariants[4],
				RevisionOrderNumber: RevisionOrderNumbers[4],
				Requester:           requesters[4],
				Revision:            gitspecs[4],
				Project:             projects[4],
			},
		}

		durationMappings := model.ProjectTaskDurations{
			map[string]*model.BuildVariantTaskDurations{
				projects[0]: &model.BuildVariantTaskDurations{
					map[string]*model.TaskDurations{
						buildVariants[0]: &model.TaskDurations{
							map[string]time.Duration{
								displayNames[0]: durations[0],
							},
						},
					},
				},
				projects[1]: &model.BuildVariantTaskDurations{
					map[string]*model.TaskDurations{
						buildVariants[1]: &model.TaskDurations{
							map[string]time.Duration{
								displayNames[1]: durations[1],
							},
						},
					},
				},
				projects[2]: &model.BuildVariantTaskDurations{
					map[string]*model.TaskDurations{
						buildVariants[2]: &model.TaskDurations{
							map[string]time.Duration{
								displayNames[2]: durations[2],
							},
						},
					},
				},
				projects[3]: &model.BuildVariantTaskDurations{
					map[string]*model.TaskDurations{
						buildVariants[3]: &model.TaskDurations{
							map[string]time.Duration{
								displayNames[3]: durations[3],
							},
						},
					},
				},
			},
		}

		So(db.Clear(model.TaskQueuesCollection), ShouldBeNil)

		Convey("saving task queues should place them in the database with the "+
			"correct ordering of tasks along with the relevant average task "+
			"completion times", func() {
			_, err := taskQueuePersister.PersistTaskQueue(distroIds[0],
				[]task.Task{tasks[0], tasks[1], tasks[2]},
				durationMappings)
			So(err, ShouldBeNil)
			_, err = taskQueuePersister.PersistTaskQueue(distroIds[1],
				[]task.Task{tasks[3], tasks[4]},
				durationMappings)
			So(err, ShouldBeNil)

			taskQueue, err := model.FindTaskQueueForDistro(distroIds[0])
			So(err, ShouldBeNil)
			So(len(taskQueue.Queue), ShouldEqual, 3)

			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[0])
			So(taskQueue.Queue[0].DisplayName, ShouldEqual,
				tasks[0].DisplayName)
			So(taskQueue.Queue[0].BuildVariant, ShouldEqual,
				tasks[0].BuildVariant)
			So(taskQueue.Queue[0].RevisionOrderNumber, ShouldEqual,
				tasks[0].RevisionOrderNumber)
			So(taskQueue.Queue[0].Requester, ShouldEqual, tasks[0].Requester)
			So(taskQueue.Queue[0].Revision, ShouldEqual, tasks[0].Revision)
			So(taskQueue.Queue[0].Project, ShouldEqual, tasks[0].Project)
			So(taskQueue.Queue[0].ExpectedDuration, ShouldEqual, durations[0])

			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[1])
			So(taskQueue.Queue[1].DisplayName, ShouldEqual,
				tasks[1].DisplayName)
			So(taskQueue.Queue[1].BuildVariant, ShouldEqual,
				tasks[1].BuildVariant)
			So(taskQueue.Queue[1].RevisionOrderNumber, ShouldEqual,
				tasks[1].RevisionOrderNumber)
			So(taskQueue.Queue[1].Requester, ShouldEqual, tasks[1].Requester)
			So(taskQueue.Queue[1].Revision, ShouldEqual, tasks[1].Revision)
			So(taskQueue.Queue[1].Project, ShouldEqual, tasks[1].Project)
			So(taskQueue.Queue[1].ExpectedDuration, ShouldEqual, durations[1])

			So(taskQueue.Queue[2].Id, ShouldEqual, taskIds[2])
			So(taskQueue.Queue[2].DisplayName, ShouldEqual,
				tasks[2].DisplayName)
			So(taskQueue.Queue[2].BuildVariant, ShouldEqual,
				tasks[2].BuildVariant)
			So(taskQueue.Queue[2].RevisionOrderNumber, ShouldEqual,
				tasks[2].RevisionOrderNumber)
			So(taskQueue.Queue[2].Requester, ShouldEqual, tasks[2].Requester)
			So(taskQueue.Queue[2].Revision, ShouldEqual, tasks[2].Revision)
			So(taskQueue.Queue[2].Project, ShouldEqual, tasks[2].Project)
			So(taskQueue.Queue[2].ExpectedDuration, ShouldEqual, durations[2])

			taskQueue, err = model.FindTaskQueueForDistro(distroIds[1])
			So(err, ShouldBeNil)
			So(len(taskQueue.Queue), ShouldEqual, 2)

			So(taskQueue.Queue[0].Id, ShouldEqual, taskIds[3])
			So(taskQueue.Queue[0].DisplayName, ShouldEqual,
				tasks[3].DisplayName)
			So(taskQueue.Queue[0].BuildVariant, ShouldEqual,
				tasks[3].BuildVariant)
			So(taskQueue.Queue[0].RevisionOrderNumber, ShouldEqual,
				tasks[3].RevisionOrderNumber)
			So(taskQueue.Queue[0].Requester, ShouldEqual, tasks[3].Requester)
			So(taskQueue.Queue[0].Revision, ShouldEqual, tasks[3].Revision)
			So(taskQueue.Queue[0].Project, ShouldEqual, tasks[3].Project)
			So(taskQueue.Queue[0].ExpectedDuration, ShouldEqual, durations[3])

			So(taskQueue.Queue[1].Id, ShouldEqual, taskIds[4])
			So(taskQueue.Queue[1].DisplayName, ShouldEqual,
				tasks[4].DisplayName)
			So(taskQueue.Queue[1].BuildVariant, ShouldEqual,
				tasks[4].BuildVariant)
			So(taskQueue.Queue[1].RevisionOrderNumber, ShouldEqual,
				tasks[4].RevisionOrderNumber)
			So(taskQueue.Queue[1].Requester, ShouldEqual, tasks[4].Requester)
			So(taskQueue.Queue[1].Revision, ShouldEqual, tasks[4].Revision)
			So(taskQueue.Queue[1].Project, ShouldEqual, tasks[4].Project)
			So(taskQueue.Queue[1].ExpectedDuration, ShouldEqual,
				model.DefaultTaskDuration)
		})

	})

}
