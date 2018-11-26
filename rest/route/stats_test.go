package route

import (
	"context"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/stretchr/testify/suite"
)

type StatsSuite struct {
	suite.Suite
}

func TestStatsSuite(t *testing.T) {
	suite.Run(t, new(StatsSuite))
}

func (s *StatsSuite) TestParseStatsFilter() {
	values := url.Values{
		"requesters":  []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
		"after_date":  []string{"1998-07-12"},
		"before_date": []string{"2018-07-15"},
		"tests":       []string{"test1", "test2"},
		"tasks":       []string{"task1", "task2"},
		"variants":    []string{"v1,v2", "v3"},
	}
	handler := testStatsHandler{}

	err := handler.parseStatsFilter(values)
	s.Require().NoError(err)

	s.Equal([]string{
		evergreen.RepotrackerVersionRequester,
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
	}, handler.filter.Requesters)
	s.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
	s.Equal(time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
	s.Equal(values["tests"], handler.filter.Tests)
	s.Equal(values["tasks"], handler.filter.Tasks)
	s.Equal([]string{"v1", "v2", "v3"}, handler.filter.BuildVariants)
	s.Nil(handler.filter.Distros)
	s.Nil(handler.filter.StartAt)
	s.Equal(stats.GroupByDistro, handler.filter.GroupBy)  // default value
	s.Equal(stats.SortEarliestFirst, handler.filter.Sort) // default value
	s.Equal(statsAPIMaxLimit+1, handler.filter.Limit)     // default value
}

func (s *StatsSuite) TestRunTestHandler() {
	var err error
	sc := &data.MockConnector{
		MockStatsConnector: data.MockStatsConnector{},
		URL:                "https://example.net/test",
	}
	handler := makeGetProjectTestStats(sc).(*testStatsHandler)
	handler.url, err = url.Parse("https://example.net/test")
	s.Require().NoError(err)

	// 100 documents will be returned
	sc.MockStatsConnector.SetTestStats("test", 100)
	handler.filter = stats.StatsFilter{Limit: 101}

	resp := handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Nil(resp.Pages())

	// 101 documents will be returned
	sc.MockStatsConnector.SetTestStats("test", 101)
	handler.filter = stats.StatsFilter{Limit: 101}

	resp = handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Pages())
	lastDoc := sc.MockStatsConnector.CachedTestStats[100]
	s.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
}

func (s *StatsSuite) TestReadTestStartAt() {
	handler := testStatsHandler{}
	startAt, err := handler.readStartAt("1998-07-12|variant1|task1|test1|distro1")
	s.Require().NoError(err)

	s.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), startAt.Date)
	s.Equal("variant1", startAt.BuildVariant)
	s.Equal("task1", startAt.Task)
	s.Equal("test1", startAt.Test)
	s.Equal("distro1", startAt.Distro)

	// Invalid format
	_, err = handler.readStartAt("1998-07-12|variant1|task1|test1")
	s.Require().Error(err)
}

func (s *StatsSuite) TestRunTaskHandler() {
	var err error
	sc := &data.MockConnector{
		MockStatsConnector: data.MockStatsConnector{},
		URL:                "https://example.net/task",
	}
	handler := makeGetProjectTaskStats(sc).(*taskStatsHandler)
	handler.url, err = url.Parse("https://example.net/test")
	s.Require().NoError(err)

	// 100 documents will be returned
	sc.MockStatsConnector.SetTaskStats("task", 100)
	handler.filter = stats.StatsFilter{Limit: 101}

	resp := handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.Nil(resp.Pages())

	// 101 documents will be returned
	sc.MockStatsConnector.SetTaskStats("task", 101)
	handler.filter = stats.StatsFilter{Limit: 101}

	resp = handler.Run(context.Background())

	s.NotNil(resp)
	s.Equal(http.StatusOK, resp.Status())
	s.NotNil(resp.Pages())
	lastDoc := sc.MockStatsConnector.CachedTaskStats[100]
	s.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
}
