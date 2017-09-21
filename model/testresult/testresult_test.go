package testresult

import (
	"fmt"
	"testing"

	"gopkg.in/mgo.v2/bson"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type TestResultSuite struct {
	suite.Suite
	tests []*TestResult
}

func TestTestResultSuite(t *testing.T) {
	suite.Run(t, new(TestResultSuite))
}

func (s *TestResultSuite) SetupSuite() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))

	s.tests = []*TestResult{}
	for i := 0; i < 5; i++ {
		s.tests = append(s.tests, &TestResult{
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

func (s *TestResultSuite) SetupTest() {
	err := db.Clear(collection)
	s.Require().NoError(err)

}

func (s *TestResultSuite) TestInsert() {

	for _, t := range s.tests {
		err := t.Insert()
		s.NoError(err)
	}

	for i, t := range s.tests {
		test, err := FindOne(db.Query(bson.M{
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
}

func (s *TestResultSuite) TestByTaskIDAndExecution() {
	for _, t := range s.tests {
		err := t.Insert()
		s.Require().NoError(err)
	}

	additionalTests := []*TestResult{}
	for i := 5; i < 10; i++ {
		additionalTests = append(additionalTests, &TestResult{
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
			TaskID:    "taskid-3",
			Execution: 3,
		})
	}

	for _, t := range additionalTests {
		err := t.Insert()
		s.NoError(err)
	}

	tests, err := Find(ByTaskIDAndExecution(fmt.Sprintf("taskid-3"), 3))
	s.NoError(err)
	s.Len(tests, 6)
	for _, test := range tests {
		s.Equal("pass", test.Status)
		s.Equal("taskid-3", test.TaskID)
		s.Equal(3, test.Execution)
	}
}
