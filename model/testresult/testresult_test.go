package testresult

import (
	"fmt"
	"testing"

	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

// FindOne returns one test result that satisfies the query. Returns nil if no tasks match.
func findOne(query db.Q) (*TestResult, error) {
	test := &TestResult{}
	err := db.FindOneQ(Collection, query, &test)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return test, err
}

type TestResultSuite struct {
	suite.Suite
	tests []TestResult
}

func TestTestResultSuite(t *testing.T) {
	suite.Run(t, new(TestResultSuite))
}

func (s *TestResultSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())

}

func (s *TestResultSuite) SetupTest() {
	err := db.Clear(Collection)
	s.Require().NoError(err)

	s.tests = []TestResult{}
	for i := 0; i < 5; i++ {
		s.tests = append(s.tests, TestResult{
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

	for _, t := range s.tests {
		err := t.Insert()
		s.Require().NoError(err)
	}

	additionalTests := []TestResult{}
	for i := 5; i < 10; i++ {
		additionalTests = append(additionalTests, TestResult{
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
			TaskID:    "taskid-5",
			Execution: 5,
		})
	}

	for _, t := range additionalTests {
		err := t.Insert()
		s.Require().NoError(err)
	}
}

func (s *TestResultSuite) TestInsertTestResultForTask() {
	taskID := "taskid-10"
	execution := 3
	i := 10
	t := TestResult{
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
	}
	err := t.InsertByTaskIDAndExecution(taskID, execution)
	s.NoError(err)
	find, err := FindByTaskIDAndExecution(taskID, execution)
	s.NoError(err)
	s.Len(find, 1)
}

func (s *TestResultSuite) TestInsertManyTestResultsForTask() {
	taskID := "taskid-25"
	execution := 3
	toInsert := []TestResult{}
	for i := 20; i < 30; i++ {
		toInsert = append(toInsert, TestResult{
			ID:        bson.NewObjectId(),
			TaskID:    taskID,
			Execution: execution,
		})
	}
	err := InsertManyByTaskIDAndExecution(toInsert, taskID, execution)
	s.NoError(err)
	find, err := FindByTaskIDAndExecution(taskID, execution)
	s.NoError(err)
	s.Len(find, 10)
}

func (s *TestResultSuite) TestInsertTestResultForTaskEmptyTaskShouldErr() {
	taskID := ""
	execution := 3
	i := 10
	t := TestResult{
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
	}
	err := t.InsertByTaskIDAndExecution(taskID, execution)
	s.Error(err)
	find, err := FindByTaskIDAndExecution(taskID, execution)
	s.NoError(err)
	s.Len(find, 0)
}

func (s *TestResultSuite) TestInsert() {
	for i, t := range s.tests {
		test, err := findOne(db.Query(bson.M{
			"_id": t.ID,
		}))
		s.NoError(err)
		s.Equal("pass", test.Status)
		s.Equal(fmt.Sprintf("file-%d", i), test.TestFile)
		s.Equal(fmt.Sprintf("url-%d", i), test.URL)
		s.Equal(fmt.Sprintf("urlraw-%d", i), test.URLRaw)
		s.Equal(fmt.Sprintf("logid-%d", i), test.LogID)
		s.Equal(i, test.LineNum)
		s.Equal(i, test.ExitCode)
		s.Equal(float64(i), test.StartTime)
		s.Equal(float64(i), test.EndTime)
		s.Equal(fmt.Sprintf("taskid-%d", i), test.TaskID)
		s.Equal(i, test.Execution)
	}

	test, err := findOne(db.Query(bson.M{
		"_id": 100,
	}))
	s.Nil(test)
	s.NoError(err)
}

func (s *TestResultSuite) TestFindByTaskIDAndExecution() {
	tests, err := FindByTaskIDAndExecution("taskid-5", 5)
	s.NoError(err)
	s.Len(tests, 5)
	for i, test := range tests {
		s.Equal("pass", test.Status)
		s.Equal(fmt.Sprintf("file-%d", i+5), test.TestFile)
		s.Equal(fmt.Sprintf("url-%d", i+5), test.URL)
		s.Equal(fmt.Sprintf("logid-%d", i+5), test.LogID)
	}

	tests, err = FindByTaskIDAndExecution("taskid-1", 1)
	s.NoError(err)
	s.Len(tests, 1)

	tests, err = FindByTaskIDAndExecution("taskid-5", 100)
	s.NoError(err)
	s.Len(tests, 0)

	tests, err = FindByTaskIDAndExecution("taskid-100", 5)
	s.NoError(err)
	s.Len(tests, 0)

	tests, err = FindByTaskIDAndExecution("taskid-100", 100)
	s.NoError(err)
	s.Len(tests, 0)
}
