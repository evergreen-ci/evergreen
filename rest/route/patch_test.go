package route

import (
	"testing"

	"time"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/stretchr/testify/suite"
)

type PatchSuite struct {
	sc        *data.MockConnector
	data      data.MockPatchConnector
	now       time.Time
	paginator PaginatorFunc

	suite.Suite
}

func TestPatchSuite(t *testing.T) {
	suite.Run(t, new(PatchSuite))
}

func (s *PatchSuite) SetupSuite() {
	s.now = time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC)
	s.data = data.MockPatchConnector{
		CachedPatches: []patch.Patch{
			{Id: "patch1", Project: "project1", CreateTime: s.now},
			{Id: "patch2", Project: "project2", CreateTime: s.now.Add(time.Second * 2)},
			{Id: "patch3", Project: "project1", CreateTime: s.now.Add(time.Second * 4)},
			{Id: "patch4", Project: "project1", CreateTime: s.now.Add(time.Second * 6)},
			{Id: "patch5", Project: "project2", CreateTime: s.now.Add(time.Second * 8)},
			{Id: "patch6", Project: "project1", CreateTime: s.now.Add(time.Second * 10)},
		},
	}
	s.paginator = patchesByProjectPaginator
	s.sc = &data.MockConnector{
		MockPatchConnector: s.data,
	}
}

func (s *PatchSuite) TestPaginatorShouldErrorIfNoResults() {
	rd, err := executeRequest("project3", s.now, 1, s.sc)
	s.Error(err)
	s.NotNil(rd)
	s.Len(rd.Result, 0)
	s.Contains(err.Error(), "no patches found")
}

func (s *PatchSuite) TestPaginatorShouldReturnResultsIfDataExists() {
	rd, err := executeRequest("project1", s.now.Add(time.Second*7), 2, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	s.Equal(model.APITime(s.now.Add(time.Second*6)), (rd.Result[0]).(*model.APIPatch).CreateTime)
	s.Equal(model.APITime(s.now.Add(time.Second*4)), (rd.Result[1]).(*model.APIPatch).CreateTime)

	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.NotNil(pageData.Prev)
	s.NotNil(pageData.Next)

	nextTime := model.NewTime(s.now).String()
	s.Equal(nextTime, pageData.Next.Key)
	prevTime := model.NewTime(s.now.Add(time.Second * 10)).String()
	s.Equal(prevTime, pageData.Prev.Key)
}

func (s *PatchSuite) TestPaginatorShouldReturnEmptyResultsIfDataIsEmpty() {
	rd, err := executeRequest("project2", s.now.Add(time.Hour), 100, s.sc)
	s.NoError(err)
	s.NotNil(rd)
	s.Len(rd.Result, 2)
	metadata, ok := rd.Metadata.(*PaginationMetadata)
	s.True(ok)
	s.NotNil(metadata)
	pageData := metadata.Pages
	s.Nil(pageData.Prev)
	s.Nil(pageData.Next)
}

func (s *PatchSuite) TestInvalidTimesAsKeyShouldError() {
	inputs := []string{
		"200096-01-02T15:04:05.000Z",
		"15:04:05.000Z",
		"invalid",
	}

	for _, i := range inputs {
		for limit := 0; limit < 3; limit++ {
			a, b, err := s.paginator(i, limit, patchesByProjectArgs{}, s.sc)
			s.Len(a, 0)
			s.Nil(b)
			s.Error(err)
			apiErr, ok := err.(rest.APIError)
			s.True(ok)
			s.Contains(apiErr.Message, i)
		}
	}
}

func executeRequest(projectId string, ts time.Time, limit int, sc *data.MockConnector) (ResponseData, error) {
	rm := getPatchesByProjectManager("", 2)
	pe := (rm.Methods[0].RequestHandler).(*patchesByProjectHandler)
	pe.Args = patchesByProjectArgs{projectId: projectId}
	pe.key = ts.Format(model.APITimeFormat)
	pe.limit = limit

	return pe.Execute(nil, sc)
}
