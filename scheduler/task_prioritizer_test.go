package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	taskComparatorTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(
		db.SessionFactoryFromConfig(taskComparatorTestConf))
	if taskComparatorTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(taskComparatorTestConf.Scheduler.LogFile)
	}
}

func TestCmpBasedTaskComparator(t *testing.T) {

	var taskComparator *CmpBasedTaskComparator
	var taskIds []string
	var tasks []task.Task

	Convey("With a CmpBasedTaskComparator", t, func() {

		taskComparator = NewCmpBasedTaskComparator()

		taskIds = []string{"t1", "t2"}

		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
		}

		alwaysEqual := func(t1, t2 task.Task, p *CmpBasedTaskComparator) (
			int, error) {
			return 0, nil
		}

		alwaysMoreImportant := func(t1, t2 task.Task,
			p *CmpBasedTaskComparator) (int, error) {
			return 1, nil
		}

		alwaysLessImportant := func(t1, t2 task.Task,
			p *CmpBasedTaskComparator) (int, error) {
			return -1, nil
		}

		idComparator := func(t1, t2 task.Task, p *CmpBasedTaskComparator) (
			int, error) {
			if t1.Id > t2.Id {
				return 1, nil
			}
			if t1.Id < t2.Id {
				return -1, nil
			}
			return 0, nil
		}

		Convey("when using the comparator functions to compare two tasks",
			func() {

				Convey("a nil comparator function slice should return the"+
					" default (no error)", func() {
					taskComparator.comparators = nil

					moreImportant, err := taskComparator.taskMoreImportantThan(
						tasks[0], tasks[1])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeFalse)

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[1], tasks[0])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeFalse)
				})

				Convey("if there are no comparator functions, the default is"+
					" always returned", func() {
					taskComparator.comparators = []taskPriorityCmp{}

					moreImportant, err := taskComparator.taskMoreImportantThan(
						tasks[0], tasks[1])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeFalse)

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[1], tasks[0])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeFalse)
				})

				Convey("if there is only one comparator function, that"+
					" function is definitive", func() {
					taskComparator.comparators = []taskPriorityCmp{
						alwaysMoreImportant,
					}

					moreImportant, err := taskComparator.taskMoreImportantThan(
						tasks[0], tasks[1])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeTrue)

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[1], tasks[0])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeTrue)
				})

				Convey("if there are multiple comparator functions, the first"+
					" definitive one wins", func() {
					taskComparator.comparators = []taskPriorityCmp{
						alwaysEqual,
						idComparator,
						alwaysMoreImportant,
						alwaysLessImportant,
					}

					moreImportant, err := taskComparator.taskMoreImportantThan(
						tasks[0], tasks[1])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeFalse)

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[1], tasks[0])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeTrue)

					// for the next two, the ids are the same so the id
					// comparator func isn't definitive

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[0], tasks[0])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeTrue)

					moreImportant, err = taskComparator.taskMoreImportantThan(
						tasks[1], tasks[1])
					So(err, ShouldBeNil)
					So(moreImportant, ShouldBeTrue)

				})

			})

	})

	Convey("Splitting tasks by requester should separate tasks based on the Requester field", t, func() {

		taskComparator = NewCmpBasedTaskComparator()

		taskIds := []string{"t1", "t2", "t3", "t4", "t5"}
		tasks := []task.Task{
			{Id: taskIds[0], Requester: evergreen.RepotrackerVersionRequester},
			{Id: taskIds[1], Requester: evergreen.PatchVersionRequester},
			{Id: taskIds[2], Requester: evergreen.PatchVersionRequester},
			{Id: taskIds[3], Requester: evergreen.RepotrackerVersionRequester},
			{Id: taskIds[4], Requester: evergreen.RepotrackerVersionRequester},
		}

		tq := taskComparator.splitTasksByRequester(tasks)
		So(len(tq.RepotrackerTasks), ShouldEqual, 3)
		repoTrackerTasks := tq.RepotrackerTasks
		So(repoTrackerTasks[0].Id, ShouldEqual, taskIds[0])
		So(repoTrackerTasks[1].Id, ShouldEqual, taskIds[3])
		So(repoTrackerTasks[2].Id, ShouldEqual, taskIds[4])
		So(len(tq.PatchTasks), ShouldEqual, 2)
		patchTasks := tq.PatchTasks
		So(patchTasks[0].Id, ShouldEqual, taskIds[1])
		So(patchTasks[1].Id, ShouldEqual, taskIds[2])

	})
	Convey("Splitting tasks with priority greater than 100 should always put those tasks in the high priority queue", t, func() {
		taskComparator = NewCmpBasedTaskComparator()

		taskIds := []string{"t1", "t2", "t3", "t4", "t5"}
		tasks := []task.Task{
			{Id: taskIds[0], Requester: evergreen.RepotrackerVersionRequester, Priority: 101},
			{Id: taskIds[1], Requester: evergreen.PatchVersionRequester, Priority: 101},
			{Id: taskIds[2], Requester: evergreen.PatchVersionRequester},
			{Id: taskIds[3], Requester: evergreen.RepotrackerVersionRequester},
			{Id: taskIds[4], Requester: evergreen.RepotrackerVersionRequester},
		}
		tq := taskComparator.splitTasksByRequester(tasks)
		So(len(tq.RepotrackerTasks), ShouldEqual, 2)
		repoTrackerTasks := tq.RepotrackerTasks
		So(repoTrackerTasks[0].Id, ShouldEqual, taskIds[3])
		So(repoTrackerTasks[1].Id, ShouldEqual, taskIds[4])
		So(len(tq.PatchTasks), ShouldEqual, 1)
		patchTasks := tq.PatchTasks
		So(patchTasks[0].Id, ShouldEqual, taskIds[2])
		So(len(tq.HighPriorityTasks), ShouldEqual, 2)
		So(tq.HighPriorityTasks[0].Id, ShouldEqual, taskIds[0])
		So(tq.HighPriorityTasks[1].Id, ShouldEqual, taskIds[1])
	})

	Convey("Merging tasks should merge the lists of repotracker and patch tasks, taking the toggle into account", t, func() {

		taskIds = []string{"t1", "t2", "t3", "t4", "t5", "t6", "t7"}
		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
			{Id: taskIds[2]},
			{Id: taskIds[3]},
			{Id: taskIds[4]},
			{Id: taskIds[5]},
			{Id: taskIds[6]},
		}

		Convey("With no patch tasks, the list of repotracker tasks should be returned", func() {
			tasks[0].Requester = evergreen.RepotrackerVersionRequester
			tasks[1].Requester = evergreen.RepotrackerVersionRequester
			tasks[2].Requester = evergreen.RepotrackerVersionRequester
			repoTrackerTasks := []task.Task{tasks[0], tasks[1], tasks[2]}
			patchTasks := []task.Task{}

			taskQueues := CmpBasedTaskQueues{
				RepotrackerTasks: repoTrackerTasks,
				PatchTasks:       patchTasks,
			}
			mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &taskQueues)
			So(len(mergedTasks), ShouldEqual, 3)
			So(mergedTasks[0].Id, ShouldEqual, taskIds[0])
			So(mergedTasks[1].Id, ShouldEqual, taskIds[1])
			So(mergedTasks[2].Id, ShouldEqual, taskIds[2])

			Convey("high priority tasks are inserted into the beginning of the queue", func() {
				tasks[3].Requester = evergreen.RepotrackerVersionRequester
				tasks[3].Priority = 101
				tasks[4].Requester = evergreen.RepotrackerVersionRequester
				tasks[4].Priority = 102

				priorityTasks := []task.Task{tasks[3], tasks[4]}
				tq := CmpBasedTaskQueues{
					HighPriorityTasks: priorityTasks,
					RepotrackerTasks:  repoTrackerTasks,
				}
				mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &tq)
				So(len(mergedTasks), ShouldEqual, 5)
				So(mergedTasks[0].Id, ShouldEqual, taskIds[3])
				So(mergedTasks[1].Id, ShouldEqual, taskIds[4])
				So(mergedTasks[2].Id, ShouldEqual, taskIds[0])
				So(mergedTasks[3].Id, ShouldEqual, taskIds[1])
				So(mergedTasks[4].Id, ShouldEqual, taskIds[2])

			})
		})

		Convey("With no repotracker tasks, the list of patch tasks should be returned", func() {
			tasks[0].Requester = evergreen.PatchVersionRequester
			tasks[1].Requester = evergreen.PatchVersionRequester
			tasks[2].Requester = evergreen.PatchVersionRequester
			repoTrackerTasks := []task.Task{}
			patchTasks := []task.Task{tasks[0], tasks[1], tasks[2]}

			taskQueues := CmpBasedTaskQueues{
				RepotrackerTasks: repoTrackerTasks,
				PatchTasks:       patchTasks,
			}
			mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &taskQueues)
			So(len(mergedTasks), ShouldEqual, 3)
			So(mergedTasks[0].Id, ShouldEqual, taskIds[0])
			So(mergedTasks[1].Id, ShouldEqual, taskIds[1])
			So(mergedTasks[2].Id, ShouldEqual, taskIds[2])
		})

		Convey("With both repotracker tasks and patch tasks, the toggle should determine how often a patch task is interleaved", func() {

			Convey("A MergeToggle of 2 should interleave tasks evenly", func() {

				taskComparatorTestConf.Scheduler.MergeToggle = 2
				repoTrackerTasks := []task.Task{tasks[0], tasks[1], tasks[2]}
				patchTasks := []task.Task{tasks[3], tasks[4], tasks[5]}

				tqs := CmpBasedTaskQueues{
					RepotrackerTasks: repoTrackerTasks,
					PatchTasks:       patchTasks,
				}
				mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &tqs)
				So(len(mergedTasks), ShouldEqual, 6)
				So(mergedTasks[0].Id, ShouldEqual, taskIds[3])
				So(mergedTasks[1].Id, ShouldEqual, taskIds[0])
				So(mergedTasks[2].Id, ShouldEqual, taskIds[4])
				So(mergedTasks[3].Id, ShouldEqual, taskIds[1])
				So(mergedTasks[4].Id, ShouldEqual, taskIds[5])
				So(mergedTasks[5].Id, ShouldEqual, taskIds[2])

			})

			Convey("A MergeToggle of 3 should interleave a patch task every third task", func() {

				taskComparatorTestConf.Scheduler.MergeToggle = 3
				repoTrackerTasks := []task.Task{tasks[0], tasks[1], tasks[2], tasks[3]}
				patchTasks := []task.Task{tasks[4], tasks[5]}

				tqs := CmpBasedTaskQueues{
					RepotrackerTasks: repoTrackerTasks,
					PatchTasks:       patchTasks,
				}
				mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &tqs)
				So(len(mergedTasks), ShouldEqual, 6)
				So(mergedTasks[0].Id, ShouldEqual, taskIds[4])
				So(mergedTasks[1].Id, ShouldEqual, taskIds[5])
				So(mergedTasks[2].Id, ShouldEqual, taskIds[0])
				So(mergedTasks[3].Id, ShouldEqual, taskIds[1])
				So(mergedTasks[4].Id, ShouldEqual, taskIds[2])
				So(mergedTasks[5].Id, ShouldEqual, taskIds[3])

			})

		})

		Convey("With a lot of patch tasks, the extras should be added on the end", func() {

			taskComparatorTestConf.Scheduler.MergeToggle = 2
			repoTrackerTasks := []task.Task{tasks[0], tasks[1]}
			patchTasks := []task.Task{tasks[2], tasks[3], tasks[4], tasks[5]}
			tqs := CmpBasedTaskQueues{
				RepotrackerTasks: repoTrackerTasks,
				PatchTasks:       patchTasks,
			}

			mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &tqs)
			So(len(mergedTasks), ShouldEqual, 6)
			So(mergedTasks[0].Id, ShouldEqual, taskIds[2])
			So(mergedTasks[1].Id, ShouldEqual, taskIds[0])
			So(mergedTasks[2].Id, ShouldEqual, taskIds[3])
			So(mergedTasks[3].Id, ShouldEqual, taskIds[1])
			So(mergedTasks[4].Id, ShouldEqual, taskIds[4])
			So(mergedTasks[5].Id, ShouldEqual, taskIds[5])
		})

		Convey("With a lot of repotracker tasks, the extras should be added on the end", func() {

			taskComparatorTestConf.Scheduler.MergeToggle = 2
			repoTrackerTasks := []task.Task{tasks[0], tasks[1], tasks[2], tasks[3], tasks[4]}
			patchTasks := []task.Task{tasks[5]}

			tqs := CmpBasedTaskQueues{
				RepotrackerTasks: repoTrackerTasks,
				PatchTasks:       patchTasks,
			}

			mergedTasks := taskComparator.mergeTasks(taskComparatorTestConf, &tqs)
			So(len(mergedTasks), ShouldEqual, 6)
			So(mergedTasks[0].Id, ShouldEqual, taskIds[5])
			So(mergedTasks[1].Id, ShouldEqual, taskIds[0])
			So(mergedTasks[2].Id, ShouldEqual, taskIds[1])
			So(mergedTasks[3].Id, ShouldEqual, taskIds[2])
			So(mergedTasks[4].Id, ShouldEqual, taskIds[3])
			So(mergedTasks[5].Id, ShouldEqual, taskIds[4])
		})

	})

}
