package route

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type TaskMetricsSuite struct {
	sc        *data.MockConnector
	data      data.MockMetricsConnector
	paginator PaginatorFunc

	procs     []*message.ProcessInfo
	timestamp string
	suite.Suite
}

func TestTaskSystemMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.paginator = taskSystemMetricsPaginator
	suite.Run(t, s)
}

func TestTaskProcessMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.paginator = taskProcessMetricsPaginator
	suite.Run(t, s)
}

func (s *TaskMetricsSuite) SetupSuite() {
	s.timestamp = time.Now().Format(model.APITimeFormat)

	for _, p := range message.CollectProcessInfoSelfWithChildren() {
		s.procs = append(s.procs, p.(*message.ProcessInfo))
	}

	for _, p := range message.CollectProcessInfoSelfWithChildren() {
		s.procs = append(s.procs, p.(*message.ProcessInfo))
	}

	s.Require().True(len(s.procs) > 0)
}

func (s *TaskMetricsSuite) SetupTest() {
	s.data = data.MockMetricsConnector{
		System: map[string][]*message.SystemInfo{
			"one": []*message.SystemInfo{
				message.CollectSystemInfo().(*message.SystemInfo),
				message.CollectSystemInfo().(*message.SystemInfo),
			},
			"two": []*message.SystemInfo{},
		},
		Process: map[string][][]*message.ProcessInfo{
			"one": [][]*message.ProcessInfo{
				s.procs,
				s.procs,
			},
			"two": [][]*message.ProcessInfo{},
		},
	}

	s.sc = &data.MockConnector{
		MockMetricsConnector: s.data,
	}
}

func (s *TaskMetricsSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
		"",
	}

	for _, i := range inputs {
		for limit := 0; limit < 20; limit++ {
			a, b, err := s.paginator(i, limit, taskMetricsArgs{}, s.sc)
			s.Len(a, 0)
			s.Nil(b)
			s.Error(err)
			apiErr, ok := err.(rest.APIError)
			s.True(ok)
			s.Contains(apiErr.Message, i)
		}
	}
}

func (s *TaskMetricsSuite) TestPaginatorShouldErrorIfNoResults() {
	// the mock errors if there isn't a matching test. the DB may
	// not error in this case.

	a, b, err := s.paginator(s.timestamp, 10, taskMetricsArgs{}, s.sc)
	s.Len(a, 0)
	s.Nil(b)
	s.Error(err)
	s.Contains(err.Error(), "database error")
}

func (s *TaskMetricsSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	a, b, err := s.paginator(s.timestamp, 100, taskMetricsArgs{"one"}, s.sc)

	s.True(len(a) > 1, "%d", len(a))
	s.NotNil(b)
	s.NoError(err)

	s.Nil(b.Next)
	s.NotNil(b.Prev)
}

func (s *TaskMetricsSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	a, b, err := s.paginator(s.timestamp, 10, taskMetricsArgs{"two"}, s.sc)

	s.True(len(a) == 0)
	s.NotNil(b)
	s.NoError(err)

	s.Nil(b.Next)
	s.Nil(b.Prev)
}

func (s *TaskMetricsSuite) TestPaginatorShouldHavePreviousPaginatedResultsWithLaterTimeStamp() {
	for i := 1; i < 3; i++ {
		a, b, err := s.paginator(s.timestamp, i, taskMetricsArgs{"one"}, s.sc)

		s.True(len(a) == i, "%d", len(a))
		s.NoError(err, "%+v", err)
		s.NotNil(b)

		if i != 2 {
			s.NotNil(b.Next)
		} else {
			s.Nil(b.Next)
		}

		s.NotNil(b.Prev)

	}
}
