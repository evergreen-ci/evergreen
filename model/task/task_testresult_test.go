package task

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
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
			Id:         fmt.Sprintf("taskid-%d", i),
			Secret:     fmt.Sprintf("secret-%d", i),
			CreateTime: time.Unix(int64(i), 0),
			Version:    fmt.Sprintf("version-%d", i),
			Project:    fmt.Sprintf("project-%d", i),
			Revision:   fmt.Sprintf("revision-%d", i),
		})
	}

	s.tests = []testresult.TestResult{}
	for i := 6; i < 10; i++ {
		s.tests = append(s.tests, testresult.TestResult{
			Status:    "pass",
			TestName:  fmt.Sprintf("file-%d", i),
			LogURL:    fmt.Sprintf("url-%d", i),
			RawLogURL: fmt.Sprintf("urlraw-%d", i),
			LineNum:   i,
			TaskID:    fmt.Sprintf("taskid-%d", i),
			Execution: i,
		})
	}
}

func (s *TaskTestResultSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(Collection, OldCollection))
	testresult.ClearLocal()

	for _, task := range s.tasks {
		s.Require().NoError(task.Insert())
	}
	testresult.InsertLocal(s.tests...)
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
		Id:             fmt.Sprintf("taskid-%d", 10),
		Secret:         fmt.Sprintf("secret-%d", 10),
		CreateTime:     time.Unix(int64(10), 0),
		Version:        fmt.Sprintf("version-%d", 10),
		Project:        fmt.Sprintf("project-%d", 10),
		Revision:       fmt.Sprintf("revision-%d", 10),
		Execution:      3,
		Status:         evergreen.TaskSucceeded,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	s.Require().NoError(t.Insert())

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			Status:    "pass",
			TestName:  fmt.Sprintf("file-%d", i),
			LogURL:    fmt.Sprintf("url-%d", i),
			RawLogURL: fmt.Sprintf("urlraw-%d", i),
			LineNum:   i,
			TaskID:    "taskid-10",
			Execution: 3,
		})
	}
	testresult.InsertLocal(testResults...)

	var err error
	t, err = FindOne(db.Query(ById("taskid-10")))
	s.Require().NoError(err)
	s.Equal("taskid-10", t.Id)
	s.Equal("secret-10", t.Secret)
	s.Equal(time.Unix(int64(10), 0), t.CreateTime)
	s.Equal("version-10", t.Version)
	s.Equal("project-10", t.Project)
	s.Equal("revision-10", t.Revision)

	s.Require().NoError(t.PopulateTestResults())
	s.Len(t.LocalTestResults, 9)
}

func (s *TaskTestResultSuite) TestArchivedTask() {
	t := &Task{
		Id:             fmt.Sprintf("taskid-%d", 20),
		Secret:         fmt.Sprintf("secret-%d", 20),
		CreateTime:     time.Unix(int64(20), 0),
		Version:        fmt.Sprintf("version-%d", 20),
		Project:        fmt.Sprintf("project-%d", 20),
		Revision:       fmt.Sprintf("revision-%d", 20),
		Execution:      3,
		Status:         evergreen.TaskFailed,
		ResultsService: testresult.TestResultsServiceLocal,
	}
	s.Require().NoError(t.Insert())
	s.Require().NoError(t.Archive())

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			Status:    "pass",
			TestName:  fmt.Sprintf("file-%d", i),
			LogURL:    fmt.Sprintf("url-%d", i),
			RawLogURL: fmt.Sprintf("urlraw-%d", i),
			LineNum:   i,
			TaskID:    "taskid-20",
			Execution: 3,
		})
	}
	testresult.InsertLocal(testResults...)

	var err error
	t, err = FindOneOld(ById("taskid-20_3"))
	s.Require().NoError(err)
	s.Equal("taskid-20_3", t.Id)
	s.Equal("secret-20", t.Secret)
	s.Equal(time.Unix(int64(20), 0), t.CreateTime)
	s.Equal("version-20", t.Version)
	s.Equal("project-20", t.Project)
	s.Equal("revision-20", t.Revision)

	s.Require().NoError(t.PopulateTestResults())
	s.Len(t.LocalTestResults, 9)
}
