package route

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type TaskMetricsSuite struct {
	routeName string
	sc        *data.MockConnector
	data      data.MockMetricsConnector
	route     gimlet.RouteHandler
	factory   func(data.Connector) gimlet.RouteHandler

	procs     []*message.ProcessInfo
	timestamp string
	suite.Suite
}

func TestTaskSystemMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.routeName = "/task/foo/metrics/system"
	s.factory = makeFetchTaskSystmMetrics
	suite.Run(t, s)
}

func TestTaskProcessMetricsSuite(t *testing.T) {
	s := new(TaskMetricsSuite)
	s.routeName = "/task/foo/metrics/process"
	s.factory = makeFetchTaskProcessMetrics
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
				message.CollectSystemInfo().(*message.SystemInfo),
				message.CollectSystemInfo().(*message.SystemInfo),
			},
			"two": []*message.SystemInfo{},
		},
		Process: map[string][][]*message.ProcessInfo{
			"one": [][]*message.ProcessInfo{
				s.procs,
				s.procs,
				s.procs,
				s.procs,
			},
			"two": [][]*message.ProcessInfo{},
		},
	}

	s.sc = &data.MockConnector{
		MockMetricsConnector: s.data,
	}
	s.sc.SetURL("https://example.evergreen.net/")
	s.route = s.factory(s.sc)
}

func (s *TaskMetricsSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
		"",
	}

	ctx := context.Background()

	for _, i := range inputs {
		for limit := 0; limit < 20; limit++ {

			r, err := http.NewRequest("GET", fmt.Sprintf("%s?start_at=%s&limit=%d", s.routeName, i, limit), nil)
			s.NoError(err)
			err = s.route.Parse(ctx, r)
			s.Error(err)
			s.Contains(err.Error(), i)
		}
	}
}

func (s *TaskMetricsSuite) TestPaginatorShouldErrorIfNoResults() {
	// the mock errors if there isn't a matching test. the DB may
	// not error in this case.

	r, err := http.NewRequest("GET", fmt.Sprintf("%s?start_at=%s&limit=%d", s.routeName, s.timestamp, 10), nil)
	ctx := r.Context()
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, r))

	resp := s.route.Run(ctx)
	s.Contains(fmt.Sprint(resp.Data()), "database error")
}

func setMetricsTask(r gimlet.RouteHandler, id string) gimlet.RouteHandler {
	switch h := r.(type) {
	case *taskSystemMetricsHandler:
		h.taskID = id
		return h
	case *taskProcessMetricsHandler:
		h.taskID = id
		return h
	default:
		panic(fmt.Sprintf("%T is not a valid metrics handler", r))
	}
}

func (s *TaskMetricsSuite) TestPaginatorShouldReturnResultsIfDataExists() {

	r, err := http.NewRequest("GET", fmt.Sprintf("%s?start_at=%s&limit=%d", s.routeName, s.timestamp, 1), nil)
	ctx := r.Context()
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, r))

	s.route = setMetricsTask(s.route, "one")

	resp := s.route.Run(ctx)

	s.True(len(resp.Data().([]interface{})) == 1, "%d", len(resp.Data().([]interface{})))
	pages := resp.Pages()
	s.NotNil(pages)

	s.Nil(pages.Prev)
	s.NotNil(pages.Next)
}

func (s *TaskMetricsSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	r, err := http.NewRequest("GET", fmt.Sprintf("%s?start_at=%s&limit=%d", s.routeName, s.timestamp, 10), nil)
	ctx := r.Context()
	s.NoError(err)
	s.NoError(s.route.Parse(ctx, r))

	s.route = setMetricsTask(s.route, "two")

	resp := s.route.Run(ctx)

	s.True(len(resp.Data().([]interface{})) == 0)
	pages := resp.Pages()
	s.Nil(pages)
}

func (s *TaskMetricsSuite) TestPaginatorShouldHavePreviousPaginatedResultsWithLaterTimeStamp() {
	for i := 1; i < 3; i++ {
		r, err := http.NewRequest("GET", fmt.Sprintf("%s?start_at=%s&limit=%d", s.routeName, s.timestamp, i), nil)
		ctx := r.Context()
		s.NoError(err)
		s.NoError(s.route.Parse(ctx, r))

		s.route = setMetricsTask(s.route, "one")

		resp := s.route.Run(ctx)
		payload := resp.Data().([]interface{})
		s.True(len(payload) == i, "%d", len(payload))
		pages := resp.Pages()

		if s.NotNil(pages, "iter=%d", i) {
			s.NotNil(pages.Next)
		}

	}
}
