package task

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TaskTestResultSuite struct {
	suite.Suite
	tasks []Task
	tests []testresult.TestResult
}

func TestTaskTestResultSuite(t *testing.T) {
	suite.Run(t, new(TaskTestResultSuite))
}

func (s *TaskTestResultSuite) SetupSuite() {
	s.tasks = []Task{}
	for i := 0; i < 5; i++ {
		s.tasks = append(s.tasks, Task{
			Id:               fmt.Sprintf("taskid-%d", i),
			Secret:           fmt.Sprintf("secret-%d", i),
			CreateTime:       time.Unix(int64(i), 0),
			Version:          fmt.Sprintf("version-%d", i),
			Project:          fmt.Sprintf("project-%d", i),
			Revision:         fmt.Sprintf("revision-%d", i),
			LocalTestResults: []TestResult{},
		})
	}

	s.tests = []testresult.TestResult{}
	for i := 6; i < 10; i++ {
		s.tests = append(s.tests, testresult.TestResult{
			ID:        mgobson.NewObjectId(),
			Status:    "pass",
			TestFile:  fmt.Sprintf("file-%d", i),
			URL:       fmt.Sprintf("url-%d", i),
			URLRaw:    fmt.Sprintf("urlraw-%d", i),
			LogID:     fmt.Sprintf("logid-%d", i),
			LineNum:   i,
			ExitCode:  i,
			StartTime: float64(i),
			EndTime:   float64(i),
			TaskID:    fmt.Sprintf("taskid-%d", i),
			Execution: i,
		})
	}
}

func (s *TaskTestResultSuite) SetupTest() {
	err := db.Clear(testresult.Collection)
	s.Require().NoError(err)
	err = db.Clear(Collection)
	s.Require().NoError(err)
	err = db.Clear(OldCollection)
	s.Require().NoError(err)

	for _, task := range s.tasks {
		err = task.Insert()
		s.Require().NoError(err)
	}
	for _, test := range s.tests {
		err = test.Insert()
		s.Require().NoError(err)
	}
}

func (s *TaskTestResultSuite) TestNoOldNoNewTestResults() {
	t, err := FindOneId(s.tasks[2].Id)
	s.NoError(err)

	s.NoError(t.PopulateTestResults())

	s.Equal("taskid-2", t.Id)
	s.Equal("secret-2", t.Secret)
	s.Equal(time.Unix(int64(2), 0), t.CreateTime)
	s.Equal("version-2", t.Version)
	s.Equal("project-2", t.Project)
	s.Equal("revision-2", t.Revision)

	s.Empty(t.LocalTestResults)
}

func (s *TaskTestResultSuite) TestNoOldNewTestResults() {
	t := &Task{
		Id:         fmt.Sprintf("taskid-%d", 10),
		Secret:     fmt.Sprintf("secret-%d", 10),
		CreateTime: time.Unix(int64(10), 0),
		Version:    fmt.Sprintf("version-%d", 10),
		Project:    fmt.Sprintf("project-%d", 10),
		Revision:   fmt.Sprintf("revision-%d", 10),
		Execution:  3,
		Status:     evergreen.TaskSucceeded,
	}
	err := t.Insert()
	s.Require().NoError(err)

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			ID:        mgobson.NewObjectId(),
			Status:    "pass",
			TestFile:  fmt.Sprintf("file-%d", i),
			URL:       fmt.Sprintf("url-%d", i),
			URLRaw:    fmt.Sprintf("urlraw-%d", i),
			LogID:     fmt.Sprintf("logid-%d", i),
			LineNum:   i,
			ExitCode:  i,
			StartTime: float64(i),
			EndTime:   float64(i),
			TaskID:    "taskid-10",
			Execution: 3,
		})
	}

	for _, r := range testResults {
		err = r.Insert()
		s.NoError(err)
	}

	t, err = FindOne(db.Query(ById("taskid-10")))
	s.NoError(err)

	s.NoError(t.PopulateTestResults())

	s.Equal("taskid-10", t.Id)
	s.Equal("secret-10", t.Secret)
	s.Equal(time.Unix(int64(10), 0), t.CreateTime)
	s.Equal("version-10", t.Version)
	s.Equal("project-10", t.Project)
	s.Equal("revision-10", t.Revision)

	s.Len(t.LocalTestResults, 9)
}

func (s *TaskTestResultSuite) TestArchivedTask() {
	t := &Task{
		Id:         fmt.Sprintf("taskid-%d", 20),
		Secret:     fmt.Sprintf("secret-%d", 20),
		CreateTime: time.Unix(int64(20), 0),
		Version:    fmt.Sprintf("version-%d", 20),
		Project:    fmt.Sprintf("project-%d", 20),
		Revision:   fmt.Sprintf("revision-%d", 20),
		Execution:  3,
		Status:     evergreen.TaskFailed,
	}
	err := t.Insert()
	s.NoError(err)
	err = t.Archive()
	s.NoError(err)

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			ID:        mgobson.NewObjectId(),
			Status:    "pass",
			TestFile:  fmt.Sprintf("file-%d", i),
			URL:       fmt.Sprintf("url-%d", i),
			URLRaw:    fmt.Sprintf("urlraw-%d", i),
			LogID:     fmt.Sprintf("logid-%d", i),
			LineNum:   i,
			ExitCode:  i,
			StartTime: float64(i),
			EndTime:   float64(i),
			TaskID:    "taskid-20",
			Execution: 3,
		})
	}

	for _, r := range testResults {
		err = r.Insert()
		s.NoError(err)
	}

	t, err = FindOneOld(ById("taskid-20_3"))
	s.NoError(err)

	s.NoError(t.PopulateTestResults())

	s.Equal("taskid-20_3", t.Id)
	s.Equal("secret-20", t.Secret)
	s.Equal(time.Unix(int64(20), 0), t.CreateTime)
	s.Equal("version-20", t.Version)
	s.Equal("project-20", t.Project)
	s.Equal("revision-20", t.Revision)

	s.Len(t.LocalTestResults, 9)
}

func TestGetTestCountByTaskIdAndFilter(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(Collection, testresult.Collection))

	numTests := 10
	numTasks := 2
	testObjects := make([]string, numTests)

	for ix := range testObjects {
		testObjects[ix] = fmt.Sprintf("TestSuite/TestNum%d", ix)
	}
	sort.StringSlice(testObjects).Sort()
	for i := 0; i < numTasks; i++ {
		id := fmt.Sprintf("task_%d", i)
		testTask := &Task{
			Id: id,
		}
		tests := make([]testresult.TestResult, numTests)
		for j := 0; j < numTests; j++ {
			status := "pass"
			if j%2 == 0 {
				status = "fail"
			}
			tests[j] = testresult.TestResult{
				TaskID:    id,
				Execution: 0,
				Status:    status,
				TestFile:  testObjects[j],
				EndTime:   float64(j),
				StartTime: 0,
			}
		}
		assert.NoError(testTask.Insert())
		for _, test := range tests {
			assert.NoError(test.Insert())
		}
	}

	for i := 0; i < numTasks; i++ {
		taskId := fmt.Sprintf("task_%d", i)
		count, err := GetTestCountByTaskIdAndFilters(taskId, "", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "", []string{"pass"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "", []string{"fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "", []string{"pass", "fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, 10)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestNum1", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, 1)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestNum2", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, 1)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{"pass", "fail"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "TestSuite/TestN", []string{"pass"}, 0)
		assert.NoError(err)
		assert.Equal(count, numTests/2)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "", []string{"pa"}, 0)
		assert.NoError(err)
		assert.Equal(count, 0)

		count, err = GetTestCountByTaskIdAndFilters(taskId, "", []string{"not_a_real_status"}, 0)
		assert.NoError(err)
		assert.Equal(count, 0)
	}
	count, err := GetTestCountByTaskIdAndFilters("fake_task", "", []string{}, 0)
	assert.Error(err)
	assert.Equal(count, 0)
}
