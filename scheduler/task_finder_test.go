package scheduler

import (
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var taskFinderTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskFinderTestConf))
	rand.Seed(time.Now().UnixNano())
}

type TaskFinderSuite struct {
	suite.Suite
	taskFinder TaskFinder
	tasks      []task.Task
	depTasks   []task.Task
}

func TestDBTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.taskFinder = &DBTaskFinder{}

	suite.Run(t, s)
}

func TestLegacyDBTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.taskFinder = &LegacyDBTaskFinder{}

	suite.Run(t, s)
}

func (s *TaskFinderSuite) SetupTest() {
	taskIds := []string{"t1", "t2", "t3", "t4"}
	s.tasks = []task.Task{
		{Id: taskIds[0], Status: evergreen.TaskUndispatched, Activated: true},
		{Id: taskIds[1], Status: evergreen.TaskUndispatched, Activated: true},
		{Id: taskIds[2], Status: evergreen.TaskUndispatched, Activated: true},
		{Id: taskIds[3], Status: evergreen.TaskUndispatched, Activated: true, Priority: -1},
	}

	depTaskIds := []string{"td1", "td2"}
	s.depTasks = []task.Task{
		{Id: depTaskIds[0]},
		{Id: depTaskIds[1]},
	}

	s.NoError(db.Clear(task.Collection))
}

func (s *TaskFinderSuite) insertTasks() {
	for _, task := range s.tasks {
		s.NoError(task.Insert())
	}
	for _, task := range s.depTasks {
		s.NoError(task.Insert())
	}
}

func (s *TaskFinderSuite) TestNoRunnableTasksReturnsEmptySlice() {
	// XXX: collection is deliberately empty
	runnableTasks, err := s.taskFinder.FindRunnableTasks()
	s.NoError(err)
	s.Empty(runnableTasks)
}

func (s *TaskFinderSuite) TestInactiveTasksNeverReturned() {
	// insert the tasks, setting one to inactive
	s.tasks[2].Activated = false
	s.insertTasks()

	// finding the runnable tasks should return two tasks
	runnableTasks, err := s.taskFinder.FindRunnableTasks()
	s.NoError(err)
	s.Len(runnableTasks, 2)
}

func (s *TaskFinderSuite) TestTasksWithUnsatisfiedDependenciesNeverReturned() {
	// edit the dependency tasks, setting one to have finished
	// successfully and one to have finished unsuccessfully
	s.depTasks[0].Status = evergreen.TaskFailed
	s.depTasks[1].Status = evergreen.TaskSucceeded

	// edit the tasks, setting one to have unmet dependencies, one to
	// have no dependencies, and one to have successfully met
	// dependencies
	s.tasks[0].DependsOn = []task.Dependency{}
	s.tasks[1].DependsOn = []task.Dependency{{s.depTasks[0].Id, evergreen.TaskSucceeded}}
	s.tasks[2].DependsOn = []task.Dependency{{s.depTasks[1].Id, evergreen.TaskSucceeded}}

	s.insertTasks()

	// finding the runnable tasks should return two tasks (the one with
	// no dependencies and the one with successfully met dependencies
	runnableTasks, err := s.taskFinder.FindRunnableTasks()
	s.NoError(err)
	s.Len(runnableTasks, 2)
}

type TaskFinderComparisonSuite struct {
	suite.Suite
	tasksGenerator   func() []task.Task
	tasks            []task.Task
	oldRunnableTasks []task.Task
	newRunnableTasks []task.Task
}

func (s *TaskFinderComparisonSuite) TestFindRunnableHostsIsIdentical() {
	idsOldMethod := []string{}
	for _, task := range s.oldRunnableTasks {
		idsOldMethod = append(idsOldMethod, task.Id)
	}

	idsNewMethod := []string{}
	for _, task := range s.newRunnableTasks {
		idsNewMethod = append(idsNewMethod, task.Id)
	}

	sort.Strings(idsOldMethod)
	sort.Strings(idsNewMethod)

	s.Equal(idsOldMethod, idsNewMethod,
		fmt.Sprintf("Failed to find identical runnable tasks; input tasks were: %+v\n", s.tasks))
}

func makeRandomTasks() []task.Task {
	tasks := []task.Task{}
	statuses := []string{
		evergreen.TaskStarted,
		evergreen.TaskUnstarted,
		evergreen.TaskUndispatched,
		evergreen.TaskDispatched,
		evergreen.TaskFailed,
		evergreen.TaskSucceeded,
		evergreen.TaskInactive,
		evergreen.TaskSystemFailed,
		evergreen.TaskTimedOut,
		evergreen.TaskSystemUnresponse,
		evergreen.TaskSystemTimedOut,
		evergreen.TaskTestTimedOut,
		evergreen.TaskConflict,
	}

	numTasks := rand.Intn(10) + 1

	for i := 0; i < numTasks; i++ {
		// pick a random status
		statusIndex := rand.Intn(len(statuses))
		id := "task" + strconv.Itoa(i)
		tasks = append(tasks, task.Task{
			Id:        id,
			Status:    statuses[statusIndex],
			Activated: true,
		})
	}

	subTasks := [][]task.Task{makeRandomSubTasks(statuses, &tasks)}

	depth := rand.Intn(3) + 1

	for i := 0; i < depth; i++ {
		subTasks = append(subTasks, makeRandomSubTasks(statuses, &subTasks[i]))
	}

	for i, _ := range subTasks {
		tasks = append(tasks, subTasks[i]...)
	}

	return tasks
}

func pickSubtaskStatus(statuses []string, dependsOn []task.Dependency) string {
	// If any task that a task depends on is undispatched, this task must be
	// undispatched
	for _, dep := range dependsOn {
		if dep.Status == evergreen.TaskUndispatched {
			return evergreen.TaskUndispatched
		}
	}
	return dependsOn[rand.Intn(len(dependsOn))].Status
}

// Add random set of dependencies to each task in parentTasks
func makeRandomSubTasks(statuses []string, parentTasks *[]task.Task) []task.Task {
	depTasks := []task.Task{}
	for i, parentTask := range *parentTasks {
		dependsOn := []task.Dependency{
			task.Dependency{
				TaskId: parentTask.Id,
				Status: parentTask.Status,
			},
		}

		// Pick another parent at random
		anotherParent := rand.Intn(len(*parentTasks))
		if anotherParent != i {
			dependsOn = append(dependsOn,
				task.Dependency{
					TaskId: (*parentTasks)[anotherParent].Id,
					Status: (*parentTasks)[anotherParent].Status,
				},
			)
		}

		numDeps := rand.Intn(4)
		for i := 0; i < numDeps; i++ {
			childId := parentTask.Id + "-child" + strconv.Itoa(i)

			depTasks = append(depTasks, task.Task{
				Id:        childId,
				Activated: true,
				Status:    pickSubtaskStatus(statuses, dependsOn),
				DependsOn: dependsOn,
			})

		}
	}

	return depTasks
}

func TestCompareTaskRunnersWithFuzzyTasks(t *testing.T) {
	s := new(TaskFinderComparisonSuite)
	s.tasksGenerator = makeRandomTasks

	suite.Run(t, s)
}

func TestCompareTaskRunnersWithStaticTasks(t *testing.T) {
	s := new(TaskFinderComparisonSuite)

	s.tasksGenerator = func() []task.Task {
		return []task.Task{
			// Successful parent
			task.Task{
				Id:        "parent0",
				Status:    evergreen.TaskSucceeded,
				Activated: true,
			},
			task.Task{
				Id:        "parent0-child0",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				// discrepancy between depends_on and the actual task's Status
				// is deliberate
				DependsOn: []task.Dependency{
					{
						TaskId: "parent0",
						Status: evergreen.TaskFailed,
					},
				},
			},

			task.Task{
				Id:        "parent0-child1",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				DependsOn: []task.Dependency{
					{
						TaskId: "parent0",
						Status: evergreen.TaskSucceeded,
					},
				},
			},

			// Failed parent
			task.Task{
				Id:        "parent1",
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
			task.Task{
				Id:        "parent1-child1-child1",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				DependsOn: []task.Dependency{
					{
						TaskId: "parent1",
						Status: evergreen.TaskFailed,
					},
				},
			},

			task.Task{
				Id:        "parent0+parent1-child0",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				DependsOn: []task.Dependency{
					{
						TaskId: "parent0",
						Status: evergreen.TaskSucceeded,
					},
					{
						TaskId: "parent1",
						Status: evergreen.TaskFailed,
					},
				},
			},

			task.Task{
				Id:        "parent2",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
			},
		}
	}

	suite.Run(t, s)
}

func (s *TaskFinderComparisonSuite) SetupTest() {
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	s.NotNil(session)
	s.NoError(db.Clear(task.Collection))

	s.tasks = s.tasksGenerator()
	s.NotEmpty(s.tasks)
	for _, task := range s.tasks {
		s.NoError(task.Insert())
	}

	oldTaskFinder := &LegacyDBTaskFinder{}
	newTaskFinder := &DBTaskFinder{}
	var err error

	s.oldRunnableTasks, err = oldTaskFinder.FindRunnableTasks()
	s.NoError(err)
	s.newRunnableTasks, err = newTaskFinder.FindRunnableTasks()
	s.NoError(err)

}
