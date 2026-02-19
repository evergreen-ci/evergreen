package scheduler

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/smartystreets/goconvey/convey/reporting"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
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
	distro            distro.Distro
	ctx               context.Context
	cancel            context.CancelFunc
}

func TestDBTaskFinder(t *testing.T) {
	s := new(TaskFinderSuite)
	s.FindRunnableTasks = func(ctx context.Context, d distro.Distro) ([]task.Task, error) {
		return task.FindHostRunnable(ctx, d.Id, true)
	}

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
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(model.ProjectRefCollection, task.Collection))
	taskIds := []string{"t0", "t1", "t2", "t3", "t4", "t5"}
	s.tasks = []task.Task{
		{Id: taskIds[0], Status: evergreen.TaskUndispatched, Activated: true, Project: "exists", CreateTime: time.Now()},
		{Id: taskIds[1], Status: evergreen.TaskUndispatched, Activated: true, Project: "exists", CreateTime: time.Now()},
		{Id: taskIds[2], Status: evergreen.TaskUndispatched, Activated: true, Project: "exists", CreateTime: time.Now()},
		{Id: taskIds[3], Status: evergreen.TaskUndispatched, Activated: true, Project: "exists", CreateTime: time.Now()},
		{Id: taskIds[4], Status: evergreen.TaskUndispatched, Activated: true, Project: "exists", CreateTime: time.Now()},
		{Id: taskIds[5], Status: evergreen.TaskUndispatched, Activated: true, Priority: -1, Project: "exists", CreateTime: time.Now()},
	}

	depTaskIds := []string{"td1", "td2"}
	s.depTasks = []task.Task{
		{Id: depTaskIds[0]},
		{Id: depTaskIds[1]},
	}

	ref := &model.ProjectRef{
		Id:      "exists",
		Enabled: true,
	}

	s.distro.PlannerSettings.Version = evergreen.PlannerVersionTunable
	s.NoError(ref.Insert(s.ctx))
}

func (s *TaskFinderSuite) TearDownTest() {
	s.NoError(db.ClearCollections(task.Collection, distro.Collection, model.ProjectRefCollection))
	s.cancel()
}

func (s *TaskFinderSuite) insertTasks() {
	for _, task := range s.tasks {
		s.NoError(task.Insert(s.ctx))
	}
	for _, task := range s.depTasks {
		s.NoError(task.Insert(s.ctx))
	}
}

func (s *TaskFinderSuite) TestNoRunnableTasksReturnsEmptySlice() {
	// XXX: collection is deliberately empty
	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Empty(runnableTasks)
}

func (s *TaskFinderSuite) TestInactiveTasksNeverReturned() {
	// insert the tasks, setting one to inactive
	s.tasks[4].Activated = false
	s.insertTasks()

	// finding the runnable tasks should return four tasks
	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Len(runnableTasks, 4)
}

func (s *TaskFinderSuite) TestFilterTasksWhenValidProjectsSet() {
	// Validate our assumption that we find 5 tasks in the default case
	s.insertTasks()
	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Len(runnableTasks, 5)

	// Validate that we find 5 tasks if their project is listed as valid
	s.Require().NoError(db.Clear(task.Collection))
	s.Require().NoError(db.Clear(distro.Collection))
	s.distro.ValidProjects = []string{"exists"}
	s.insertTasks()
	s.Require().NoError(s.distro.Insert(s.ctx))
	runnableTasks, err = s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Len(runnableTasks, 5)

	// Change some projects and validate we don't find those
	s.Require().NoError(db.Clear(task.Collection))
	s.Require().NoError(db.Clear(distro.Collection))
	s.tasks[0].Project = "something_else"
	s.tasks[1].Project = "something_else"
	s.insertTasks()
	runnableTasks, err = s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Len(runnableTasks, 3)
}

func (s *TaskFinderSuite) TestTasksWithUnsatisfiedDependenciesNeverReturned() {
	// edit the dependency tasks, setting one to have not finished
	// and one to have failed
	s.depTasks[0].Status = evergreen.TaskFailed
	s.depTasks[1].Status = evergreen.TaskUndispatched
	s.depTasks[1].DependsOn = []task.Dependency{
		{
			TaskId:       "none",
			Status:       "*",
			Unattainable: true,
		},
	}

	// Matching dependency - runnable
	s.tasks[0].DependsOn = []task.Dependency{{TaskId: s.depTasks[0].Id, Status: evergreen.TaskFailed}}
	// Not matching - not runnable
	s.tasks[1].DependsOn = []task.Dependency{{TaskId: s.depTasks[0].Id, Status: evergreen.TaskSucceeded}}
	// Dependent task 1 is blocked and status is "*" - runnable.
	// Also demonstrates two satisfied dependencies
	s.tasks[2].DependsOn = []task.Dependency{{TaskId: s.depTasks[1].Id, Status: "*"}, {TaskId: s.depTasks[0].Id, Status: "*"}}
	// * status matches any finished status - runnable
	s.tasks[3].DependsOn = []task.Dependency{{TaskId: s.depTasks[0].Id, Status: "*"}}

	s.insertTasks()

	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Len(runnableTasks, 4)
	expectedRunnableTasks := []string{"t0", "t2", "t3", "t4"}
	for _, t := range runnableTasks {
		s.Contains(expectedRunnableTasks, t.Id)
	}
}

func (s *TaskFinderSuite) TestTasksWithDisabledProjectNeverReturned() {
	ref := &model.ProjectRef{
		Id:      "exists",
		Enabled: false,
	}
	s.Require().NoError(ref.Replace(s.ctx))
	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Empty(runnableTasks)
}

func (s *TaskFinderSuite) TestTasksWithProjectDispatchingDisabledNeverReturned() {
	ref := &model.ProjectRef{
		Id:                  "exists",
		DispatchingDisabled: utility.TruePtr(),
	}
	s.Require().NoError(ref.Replace(s.ctx))
	runnableTasks, err := s.FindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)
	s.Empty(runnableTasks)
}

type TaskFinderComparisonSuite struct {
	suite.Suite
	tasksGenerator   func() []task.Task
	distro           distro.Distro
	tasks            []task.Task
	oldRunnableTasks []task.Task
	newRunnableTasks []task.Task
	altRunnableTasks []task.Task
	pllRunnableTasks []task.Task

	ctx    context.Context
	cancel context.CancelFunc
}

func (s *TaskFinderComparisonSuite) SetupSuite() {
	s.NoError(db.Clear(model.ProjectRefCollection))

	ref := &model.ProjectRef{
		Id:      "exists",
		Enabled: true,
	}
	s.NoError(ref.Insert(s.ctx))

	ref = &model.ProjectRef{
		Id:      "disabled",
		Enabled: false,
	}

	s.NoError(ref.Insert(s.ctx))

	ref = &model.ProjectRef{
		Id:               "patching-disabled",
		PatchingDisabled: utility.TruePtr(),
		Enabled:          true,
	}

	s.NoError(ref.Insert(s.ctx))

	ref = &model.ProjectRef{
		Id:                  "dispatching-disabled",
		DispatchingDisabled: utility.TruePtr(),
		Enabled:             true,
	}

	s.NoError(ref.Insert(s.ctx))

	s.distro.PlannerSettings.Version = evergreen.PlannerVersionTunable
}

func (s *TaskFinderComparisonSuite) TearDownSuite() {
	s.NoError(db.Clear(model.ProjectRefCollection))
	s.cancel()
}

func (s *TaskFinderComparisonSuite) SetupTest() {
	s.NoError(db.Clear(task.Collection))

	s.ctx, s.cancel = context.WithCancel(context.Background())
	env := evergreen.GetEnvironment()
	_, err := env.DB().Collection(task.Collection).Indexes().CreateMany(s.ctx, []mongo.IndexModel{
		{
			Keys: bson.D{{Key: task.ActivatedKey, Value: 1}, {Key: task.StatusKey, Value: 1}, {Key: task.PriorityKey, Value: 1}},
		},
		{
			Keys: bson.D{{Key: bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyTaskIdKey), Value: 1}},
		},
	})
	s.Require().NoError(err)

	s.tasks = s.tasksGenerator()
	s.NotEmpty(s.tasks)
	for _, task := range s.tasks {
		task.BuildVariant = "aBuildVariant"
		task.Tags = []string{"tag1", "tag2"}
		s.NoError(task.Insert(s.ctx))
	}

	s.newRunnableTasks, err = RunnableTasksPipeline(s.ctx, s.distro)
	s.NoError(err)

	s.oldRunnableTasks, err = LegacyFindRunnableTasks(s.ctx, s.distro)
	s.NoError(err)

	s.altRunnableTasks, err = AlternateTaskFinder(s.ctx, s.distro)
	s.NoError(err)

	s.pllRunnableTasks, err = ParallelTaskFinder(s.ctx, s.distro)
	s.NoError(err)
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
		s.Equal("aBuildVariant", task.BuildVariant)
		s.Equal([]string{"tag1", "tag2"}, task.Tags)
	}
	for _, task := range s.newRunnableTasks {
		s.Equal("aBuildVariant", task.BuildVariant)
		s.Equal([]string{"tag1", "tag2"}, task.Tags)
	}
	for _, task := range s.altRunnableTasks {
		s.Equal("aBuildVariant", task.BuildVariant)
		s.Equal([]string{"tag1", "tag2"}, task.Tags)
	}
	for _, task := range s.pllRunnableTasks {
		s.Equal("aBuildVariant", task.BuildVariant)
		s.Equal([]string{"tag1", "tag2"}, task.Tags)
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
			{
				Id:        "parent0",
				Status:    evergreen.TaskSucceeded,
				Activated: true,
			},
			{
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

			{
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
			{
				Id:        "parent1",
				Status:    evergreen.TaskFailed,
				Activated: true,
			},
			{
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

			{
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

			{
				Id:        "parent2",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
			},
			{
				Id:        "foo",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				Project:   "disabled",
			},
			{
				Id:        "bar",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				Requester: evergreen.PatchVersionRequester,
				Project:   "patching-disabled",
			},
			{
				Id:        "baz",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				Requester: evergreen.GithubPRRequester,
				Project:   "patching-disabled",
			},
			{
				Id:        "runnable",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				Requester: evergreen.RepotrackerVersionRequester,
				Project:   "patching-disabled",
			},
			{
				Id:        "also-runnable",
				Status:    evergreen.TaskUndispatched,
				Activated: true,
				Requester: evergreen.RepotrackerVersionRequester,
				Project:   "dispatching-disabled",
			},
		}
	}

	suite.Run(t, s)
}

func makeRandomTasks() []task.Task {
	tasks := []task.Task{}
	statuses := []string{
		evergreen.TaskStarted,
		evergreen.TaskUnscheduled,
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
	}

	numTasks := rand.Intn(10) + 10
	for i := 0; i < numTasks; i++ {
		// pick a random status
		statusIndex := rand.Intn(len(statuses))
		id := "task" + strconv.Itoa(i)
		tasks = append(tasks, task.Task{
			Id:        id,
			Status:    statuses[statusIndex],
			Activated: true,
			Project:   "exists",
		})
	}

	subTasks := [][]task.Task{makeRandomSubTasks(&tasks)}

	depth := rand.Intn(6) + 1

	for i := 0; i < depth; i++ {
		subTasks = append(subTasks, makeRandomSubTasks(&subTasks[i]))
	}

	for i := range subTasks {
		tasks = append(tasks, subTasks[i]...)
	}

	tasks = append(tasks, task.Task{
		Id:        "doesn't exist task name",
		Status:    evergreen.TaskUndispatched,
		Activated: true,
		Project:   "doesn't exist",
	})

	return tasks
}

func pickSubtaskStatus(dependsOn []task.Dependency) string {
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
func makeRandomSubTasks(parentTasks *[]task.Task) []task.Task {
	depTasks := []task.Task{}
	for i, parentTask := range *parentTasks {
		dependsOn := []task.Dependency{
			{
				TaskId: parentTask.Id,
				Status: getRandomDependsOnStatus(),
			},
		}

		// Pick another parent at random
		anotherParent := rand.Intn(len(*parentTasks))
		if anotherParent != i {
			dependsOn = append(dependsOn,
				task.Dependency{
					TaskId: (*parentTasks)[anotherParent].Id,
					Status: getRandomDependsOnStatus(),
				},
			)
		}

		numDeps := rand.Intn(6)
		for i := 0; i < numDeps; i++ {
			childId := parentTask.Id + "-child" + strconv.Itoa(i)

			depTasks = append(depTasks, task.Task{
				Id:        childId,
				Activated: true,
				Status:    pickSubtaskStatus(dependsOn),
				DependsOn: dependsOn,
				Project:   "exists",
			})

		}
	}

	return depTasks
}

func getRandomDependsOnStatus() string {
	dependsOnStatuses := []string{evergreen.TaskSucceeded, evergreen.TaskFailed, task.AllStatuses}
	return dependsOnStatuses[rand.Intn(len(dependsOnStatuses))]
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
				Project:   "exists",
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
				Project:   "exists",
			})
		}

		tasks = append(tasks, task.Task{
			Id:        "skipped-project00",
			Status:    evergreen.TaskUndispatched,
			Activated: true,
			Project:   "doesn't exist",
		})

		return tasks
	}
	suite.Run(t, s)
}
