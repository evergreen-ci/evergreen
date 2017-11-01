package scheduler

import (
	"bytes"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
)

func init() {
	rand.Seed(time.Now().UnixNano())
	reporting.QuietMode()
}

type TaskFinderSuite struct {
	suite.Suite
	FindRunnableTasks TaskFinder
	tasks             []task.Task
	depTasks          []task.Task
}

func TestDBTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.FindRunnableTasks = task.FindRunnable

	suite.Run(t, s)
}

func TestLegacyDBTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.FindRunnableTasks = LegacyFindRunnableTasks
	suite.Run(t, s)
}

func TestAlternativeTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.FindRunnableTasks = AlternateTaskFinder

	suite.Run(t, s)
}

func TestParallelTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.FindRunnableTasks = ParallelTaskFinder

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

func (s *TaskFinderSuite) TearDownTest() {
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
	runnableTasks, err := s.FindRunnableTasks()
	s.NoError(err)
	s.Empty(runnableTasks)
}

func (s *TaskFinderSuite) TestInactiveTasksNeverReturned() {
	// insert the tasks, setting one to inactive
	s.tasks[2].Activated = false
	s.insertTasks()

	// finding the runnable tasks should return two tasks
	runnableTasks, err := s.FindRunnableTasks()
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
	s.tasks[1].DependsOn = []task.Dependency{{TaskId: s.depTasks[0].Id, Status: evergreen.TaskSucceeded}}
	s.tasks[2].DependsOn = []task.Dependency{{TaskId: s.depTasks[1].Id, Status: evergreen.TaskSucceeded}}

	s.insertTasks()

	// finding the runnable tasks should return two tasks (the one with
	// no dependencies and the one with successfully met dependencies
	runnableTasks, err := s.FindRunnableTasks()
	s.NoError(err)
	s.Len(runnableTasks, 2)
}

type TaskFinderComparisonSuite struct {
	suite.Suite
	tasksGenerator   func() []task.Task
	tasks            []task.Task
	oldRunnableTasks []task.Task
	newRunnableTasks []task.Task
	altRunnableTasks []task.Task
	pllRunnableTasks []task.Task
}

func (s *TaskFinderComparisonSuite) SetupTest() {
	session, mdb, err := db.GetGlobalSessionFactory().GetSession()
	s.NoError(err)
	s.NotNil(session)
	defer session.Close()
	s.NoError(db.Clear(task.Collection))

	s.NoError(mdb.C(task.Collection).EnsureIndexKey("activated", "status", "priority"))
	s.NoError(mdb.C(task.Collection).EnsureIndexKey("depends_on._id"))

	s.tasks = s.tasksGenerator()
	s.NotEmpty(s.tasks)
	for _, task := range s.tasks {
		task.BuildVariant = "aBuildVariant"
		task.Tags = []string{"tag1", "tag2"}
		s.NoError(task.Insert())
	}

	grip.Info("start new")
	s2start := time.Now()
	s.newRunnableTasks, err = FindRunnableTasks()
	s2dur := time.Since(s2start)
	s.NoError(err)
	grip.Info("end db")

	grip.Info("start legacy")
	s1start := time.Now()
	s.oldRunnableTasks, err = LegacyFindRunnableTasks()
	s1dur := time.Since(s1start)
	s.NoError(err)
	grip.Info("end legacy")

	grip.Info("start alternate")
	s3start := time.Now()
	s.altRunnableTasks, err = AlternateTaskFinder()
	s3dur := time.Since(s3start)
	s.NoError(err)
	grip.Info("end alt")

	grip.Info("start parallel")
	s4start := time.Now()
	s.pllRunnableTasks, err = ParallelTaskFinder()
	s4dur := time.Since(s4start)
	s.NoError(err)
	grip.Info("end parallel")

	grip.Notice(message.Fields{
		"alternative": s3dur.String(),
		"legacy":      s1dur.String(),
		"length":      len(s.tasks),
		"parallel":    s4dur.String(),
		"pipeline":    s2dur.String(),
	})
}

func (s *TaskFinderComparisonSuite) TearDownTest() {
	s.NoError(db.Clear(task.Collection))
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

	idsAltMethod := []string{}
	for _, task := range s.altRunnableTasks {
		idsAltMethod = append(idsAltMethod, task.Id)
	}

	idsPllMethod := []string{}
	for _, task := range s.pllRunnableTasks {
		idsPllMethod = append(idsPllMethod, task.Id)
	}

	sort.Strings(idsOldMethod)
	sort.Strings(idsNewMethod)
	sort.Strings(idsAltMethod)
	sort.Strings(idsPllMethod)

	s.Equal(idsOldMethod, idsNewMethod, "old (legacy) and new (database) methods did not match")
	s.Equal(idsOldMethod, idsAltMethod, "old (legacy) and new (altimpl) methods did not match")
	s.Equal(idsNewMethod, idsAltMethod, "new (database) and new (altimpl) methods did not match")
	s.Equal(idsNewMethod, idsPllMethod, "new (database) and new (parallel) methods did not match")
}

func (s *TaskFinderComparisonSuite) TestCheckThatTaskIsPopulated() {
	for _, task := range s.oldRunnableTasks {
		s.Equal(task.BuildVariant, "aBuildVariant")
		s.Equal(task.Tags, []string{"tag1", "tag2"})
	}
	for _, task := range s.newRunnableTasks {
		s.Equal(task.BuildVariant, "aBuildVariant")
		s.Equal(task.Tags, []string{"tag1", "tag2"})
	}
	for _, task := range s.altRunnableTasks {
		s.Equal(task.BuildVariant, "aBuildVariant")
		s.Equal(task.Tags, []string{"tag1", "tag2"})
	}
	for _, task := range s.pllRunnableTasks {
		s.Equal(task.BuildVariant, "aBuildVariant")
		s.Equal(task.Tags, []string{"tag1", "tag2"})
	}
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

	numTasks := rand.Intn(20) + 20
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

	depth := rand.Intn(6) + 1

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

		numDeps := rand.Intn(8)
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

func hugeString(suffix string) string {
	var buffer bytes.Buffer
	// 4 megabytes
	for i := 0; i < 4*1000*1000; i++ {
		buffer.WriteString("a")
	}
	buffer.WriteString(suffix)
	return buffer.String()
}

func TestCompareTaskRunnersWithHugeTasks(t *testing.T) {
	s := new(TaskFinderComparisonSuite)

	s.tasksGenerator = func() []task.Task {
		tasks := []task.Task{
			{
				Id:        "hugedeps",
				Status:    evergreen.TaskUndispatched,
				OldTaskId: hugeString("huge"),
				Activated: true,
			},
		}

		// tasks[0] will depend on 5 other tasks, each with a
		// 4 megabyte string inside of it. After graphLookup, the
		// intermediate document will have 24 megabytes of data in it.
		for i := 0; i < 5; i++ {
			taskName := fmt.Sprintf("task%d", i)
			tasks[0].DependsOn = append(tasks[0].DependsOn, task.Dependency{
				TaskId: taskName,
				Status: evergreen.TaskSucceeded,
			})

			tasks = append(tasks, task.Task{
				Id:        taskName,
				OldTaskId: hugeString(fmt.Sprintf("%d", i)),
				Status:    evergreen.TaskSucceeded,
				Activated: true,
			})
		}

		return tasks
	}
	suite.Run(t, s)
}
