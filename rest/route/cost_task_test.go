package route

import (
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
	sc        *data.MockConnector
	data      data.MockTaskConnector
	starttime time.Time
	paginator PaginatorFunc
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
		MockTaskConnector: s.data,
	}
}

func (s *costTasksByProjectSuite) TestPaginatorShouldErrorIfNoResults() {
	rd, err := executeCostTasksByProjectRequest("fake_project", "", s.starttime, time.Duration(1), 1, s.sc)
	s.NoError(err) // Test queries that will return no results should not raise errors
	s.NotNil(rd)
	s.Len(rd.Result, 0)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataNoPagination() {
	// time range + limit covers all three tasks; no pagination necessary
	rd, err := executeCostTasksByProjectRequest("project", "", s.starttime, time.Duration(5000), 3, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 3)
	s.Equal(model.APIString("task1"), (rd.Result[0]).(*model.APITaskCost).Id)
	s.Equal(model.APIString("task2"), (rd.Result[1]).(*model.APITaskCost).Id)
	s.Equal(model.APIString("task3"), (rd.Result[2]).(*model.APITaskCost).Id)
	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.Nil(pageData.Prev)
	s.Nil(pageData.Next)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataNextPage() {
	// time range covers first two tasks; limit limits to returning one of them;
	// the other task must be in the next page
	rd, err := executeCostTasksByProjectRequest("project", "", s.starttime, time.Duration(3000), 1, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 1)
	s.Equal(model.APIString("task1"), (rd.Result[0]).(*model.APITaskCost).Id)
	metadata, _ := rd.Metadata.(*PaginationMetadata)
	pageData := metadata.Pages
	s.NotNil(pageData.Next)
	s.Equal("task2", pageData.Next.Key)
	s.Nil(pageData.Prev)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataPrevPage() {
	// time range covers all three tasks; query starts at the second task so two
	// tasks should be returned; the first task must be in the prev page
	rd, err := executeCostTasksByProjectRequest("project", "task2", s.starttime, time.Duration(5000), 2, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	s.Equal(model.APIString("task2"), (rd.Result[0]).(*model.APITaskCost).Id)
	s.Equal(model.APIString("task3"), (rd.Result[1]).(*model.APITaskCost).Id)
	metadata, _ := rd.Metadata.(*PaginationMetadata)
	pageData := metadata.Pages
	s.Nil(pageData.Next)
	s.NotNil(pageData.Prev)
	s.Equal("task1", pageData.Prev.Key)
}

func (s *costTasksByProjectSuite) TestPaginatorReturnDataPrevPageKey() {
	// time range covers all three tasks; query starts at the third task so one
	// task should be returned; the first two tasks must be in the prev page
	// and the first task should be the key
	rd, err := executeCostTasksByProjectRequest("project", "task3", s.starttime, time.Duration(5000), 2, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 1)
	s.Equal(model.APIString("task3"), (rd.Result[0]).(*model.APITaskCost).Id)
	metadata, _ := rd.Metadata.(*PaginationMetadata)
	pageData := metadata.Pages
	s.Nil(pageData.Next)
	s.NotNil(pageData.Prev)
	s.Equal("task1", pageData.Prev.Key)
}

// If the given parameters yield no results, it should merely return an empty
// Result in the ResponseData. There should be no errors or nil ResponseData.
func (s *costTasksByProjectSuite) TestPaginatorNoData() {
	rd, err := executeCostTasksByProjectRequest("fake", "", s.starttime, time.Duration(5000), 3, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 0)
}

func executeCostTasksByProjectRequest(project, key string, starttime time.Time,
	duration time.Duration, limit int, sc *data.MockConnector) (ResponseData, error) {
	rm := getCostTaskByProjectRouteManager("", 2)
	handler := (rm.Methods[0].RequestHandler).(*costTasksByProjectHandler)
	handler.Args = costTasksByProjectArgs{projectID: project, starttime: starttime, duration: duration}
	handler.limit = limit
	handler.key = key
	return handler.Execute(nil, sc)
}
