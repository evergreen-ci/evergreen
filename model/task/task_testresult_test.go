package task

import (
	"fmt"
	"testing"
	"time"

	"gopkg.in/mgo.v2/bson"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/testutil"
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
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

	s.tasks = []Task{}
	for i := 0; i < 5; i++ {
		s.tasks = append(s.tasks, Task{
			Id:          fmt.Sprintf("taskid-%d", i),
			Secret:      fmt.Sprintf("secret-%d", i),
			CreateTime:  time.Unix(int64(i), 0),
			Version:     fmt.Sprintf("version-%d", i),
			Project:     fmt.Sprintf("project-%d", i),
			Revision:    fmt.Sprintf("revision-%d", i),
			TestResults: []TestResult{},
		})
	}

	s.tests = []testresult.TestResult{}
	for i := 6; i < 10; i++ {
		s.tests = append(s.tests, testresult.TestResult{
			ID:        bson.NewObjectId(),
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
	t, err := FindOneNoMerge(ById(s.tasks[2].Id))
	s.NoError(err)

	err = t.MergeNewTestResults()
	s.NoError(err)

	s.Equal("taskid-2", t.Id)
	s.Equal("secret-2", t.Secret)
	s.Equal(time.Unix(int64(2), 0), t.CreateTime)
	s.Equal("version-2", t.Version)
	s.Equal("project-2", t.Project)
	s.Equal("revision-2", t.Revision)

	s.Empty(t.TestResults)
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
	}
	err := t.Insert()
	s.Require().NoError(err)

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			ID:        bson.NewObjectId(),
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

	t, err = FindOneNoMerge(ById("taskid-10"))
	s.NoError(err)

	err = t.MergeNewTestResults()
	s.NoError(err)

	s.Equal("taskid-10", t.Id)
	s.Equal("secret-10", t.Secret)
	s.Equal(time.Unix(int64(10), 0), t.CreateTime)
	s.Equal("version-10", t.Version)
	s.Equal("project-10", t.Project)
	s.Equal("revision-10", t.Revision)

	s.Len(t.TestResults, 9)
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
	}
	err := t.Insert()
	s.NoError(err)
	err = t.Archive()
	s.NoError(err)

	testResults := []testresult.TestResult{}
	for i := 11; i < 20; i++ {
		testResults = append(testResults, testresult.TestResult{
			ID:        bson.NewObjectId(),
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

	t, err = FindOneOldNoMerge(ById("taskid-20_3"))
	s.NoError(err)

	err = t.MergeNewTestResults()
	s.NoError(err)

	s.Equal("taskid-20_3", t.Id)
	s.Equal("secret-20", t.Secret)
	s.Equal(time.Unix(int64(20), 0), t.CreateTime)
	s.Equal("version-20", t.Version)
	s.Equal("project-20", t.Project)
	s.Equal("revision-20", t.Revision)

	s.Len(t.TestResults, 9)
}
