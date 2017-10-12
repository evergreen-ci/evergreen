package scheduler

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var taskFinderTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(taskFinderTestConf))
}

type DBTaskFinderSuite struct {
	suite.Suite
	taskFinder *DBTaskFinder
	tasks      []task.Task
	depTasks   []task.Task
}

func TestDBTaskFinder(t *testing.T) {
	s := new(DBTaskFinderSuite)

	suite.Run(t, s)
}

func (s *DBTaskFinderSuite) SetupTest() {
	s.taskFinder = &DBTaskFinder{}

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

	s.Nil(db.Clear(task.Collection))
}

func (s *DBTaskFinderSuite) insertTasks() {
	for _, task := range s.tasks {
		s.Nil(task.Insert())
	}
	for _, task := range s.depTasks {
		s.Nil(task.Insert())
	}
}

func (s *DBTaskFinderSuite) TestNoRunnableTasksReturnsEmptySlice() {
	// XXX: collection is empty without running insertTasks
	runnableTasks, err := s.taskFinder.FindRunnableTasks()
	s.Nil(err)
	s.Empty(runnableTasks)
}

func (s *DBTaskFinderSuite) TestInactiveTasksNeverReturned() {
	// insert the tasks, setting one to inactive
	s.tasks[2].Activated = false
	s.insertTasks()

	// finding the runnable tasks should return two tasks
	runnableTasks, err := s.taskFinder.FindRunnableTasks()
	s.Nil(err)
	s.Len(runnableTasks, 2)
}

func (s *DBTaskFinderSuite) TestTasksWithUnsatisfiedDependenciesNeverReturned() {

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
	s.Nil(err)
	s.Len(runnableTasks, 2)
}

type TaskFinderComparisonSuite struct {
	suite.Suite
}

func TestTaskFinderComparisonSuite(t *testing.T) {
	s := new(TaskFinderComparisonSuite)

	suite.Run(t, s)
}

func (s *TaskFinderComparisonSuite) SetupTest() {
	session, _, _ := db.GetGlobalSessionFactory().GetSession()
	s.NotNil(session)
	s.NoError(session.DB(taskFinderTestConf.Database.DB).DropDatabase())

	for _, task := range tasks {
		s.NoError(task.Insert())
	}
}

func (s *TaskFinderComparisonSuite) TestFindRunnableHostsIsIdentical() {
	taskFinder := &DBTaskFinder{}

	oldRunnableTasks, err := taskFinder.findRunnableTasks()
	s.Nil(err)
	newRunnableTasks, err1 := taskFinder.FindRunnableTasks()
	s.Nil(err1)

	s.NotEmpty(oldRunnableTasks)
	s.NotEmpty(newRunnableTasks)

	s.Equal(oldRunnableTasks, newRunnableTasks)
}

// A suitably complex set of tasks for comparing the old method to the new method
var tasks = []task.Task{
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
		Id:        "parent0-child0-child0",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "parent0-child0",
				Status: evergreen.TaskUndispatched,
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
	// task with no status in depends_on
	task.Task{
		Id:        "parent0-child2",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "parent0",
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
		Id:        "parent1-child0",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "parent1",
				Status: evergreen.TaskFailed,
			},
		},
	},

	// other parent
	task.Task{
		Id:        "parent2",
		Status:    evergreen.TaskSystemFailed,
		Activated: true,
	},
	task.Task{
		Id:        "parent2-child0",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "parent2",
				Status: evergreen.TaskUndispatched,
			},
		},
	},

	// parent with negative priority
	task.Task{
		Id:        "parent3",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Priority:  -1,
	},
	task.Task{
		Id:        "parent3-child1",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		DependsOn: []task.Dependency{
			{
				TaskId: "parent3",
				Status: evergreen.TaskUndispatched,
			},
		},
	},

	// undispatched task with no dependencies
	task.Task{
		Id:        "parent4",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
	},

	// undispatched, inactive task
	task.Task{
		Id:        "parent5",
		Status:    evergreen.TaskUndispatched,
		Activated: false,
	},
}
