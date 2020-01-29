package route

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

//-------------------------------------------------------------------------//
//   Tests for FindCostTaskByProject route                                 //
//-------------------------------------------------------------------------//
type costTasksByProjectSuite struct {
	route     *costTasksByProjectHandler
	sc        *data.MockConnector
	data      data.MockTaskConnector
	starttime time.Time
	suite.Suite
}

func TestCostTaskByProjectSuite(t *testing.T) {
	suite.Run(t, new(costTasksByProjectSuite))
}

func (s *costTasksByProjectSuite) SetupSuite() {
	s.starttime = time.Now()

	testTask1 := task.Task{
		Id:           "task1",
		DisplayName:  "displname1",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(1000)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}

	testTask2 := task.Task{
		Id:           "task2",
		DisplayName:  "displname2",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(2500)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}
	testTask3 := task.Task{
		Id:           "task3",
		DisplayName:  "displname3",
		DistroId:     "distro",
		BuildVariant: "bv",
		FinishTime:   s.starttime.Add(time.Duration(5000)),
		TimeTaken:    time.Duration(1),
		Project:      "project",
		Revision:     "githash",
	}

	s.data = data.MockTaskConnector{
		CachedTasks: []task.Task{testTask1, testTask2, testTask3},
	}
	s.sc = &data.MockConnector{
		URL:               "https://evergreen.example.net",
		MockTaskConnector: s.data,
	}
}

func (s *costTasksByProjectSuite) SetupTest() {
	s.route = &costTasksByProjectHandler{
		starttime: s.starttime,
		sc:        s.sc,
	}
}

func (s *costTasksByProjectSuite) TestPaginatorShouldErrorIfNoResults() {
	s.route.projectID = "fake_project"
	s.route.duration = 1
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.NotEqual(resp.Status(), http.StatusOK, "%+v", resp.Data())
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataNoPagination() {
	// time range + limit covers all three tasks; no pagination necessary
	s.route.projectID = "project"
	s.route.duration = time.Duration(5000)
	s.route.limit = 3

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})
	s.Len(payload, 3)
	s.Equal(model.ToStringPtr("task1"), (payload[0]).(*model.APITaskCost).Id)
	s.Equal(model.ToStringPtr("task2"), (payload[1]).(*model.APITaskCost).Id)
	s.Equal(model.ToStringPtr("task3"), (payload[2]).(*model.APITaskCost).Id)
	pages := resp.Pages()
	s.Nil(pages)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataNextPage() {
	// time range covers first two tasks; limit limits to returning one of them;
	// the other task must be in the next page
	s.route.projectID = "project"
	s.route.duration = time.Duration(3000)
	s.route.limit = 1

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})
	s.Len(payload, 1)
	s.Equal(model.ToStringPtr("task1"), (payload[0]).(*model.APITaskCost).Id)

	pages := resp.Pages()
	s.NotNil(pages)
	s.Nil(pages.Prev)
	s.NotNil(pages.Next)
	s.Equal("task2", pages.Next.Key)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataPrevPageKey() {
	// time range covers all three tasks; query starts at the third task so one
	// task should be returned; the first two tasks must be in the prev page
	// and the first task should be the key
	s.route.projectID = "project"
	s.route.duration = time.Duration(5000)
	s.route.key = "task3"
	s.route.limit = 2

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	payload := resp.Data().([]interface{})
	s.Len(payload, 1)
	s.Equal(model.ToStringPtr("task3"), (payload[0]).(*model.APITaskCost).Id)
}

// If the given parameters yield no results, it should merely return an empty
// Result in the ResponseData. There should be no errors or nil ResponseData.
func (s *costTasksByProjectSuite) TestPaginatorNoData() {
	s.route.projectID = "fake"
	s.route.duration = time.Duration(5000)
	s.route.limit = 3

	resp := s.route.Run(context.Background())
	s.NotNil(resp)
	s.Equal(http.StatusNotFound, resp.Status(), "%+v", resp.Data())
}
