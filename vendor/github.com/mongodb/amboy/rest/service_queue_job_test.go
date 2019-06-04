// +build go1.7

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type JobStatusSuite struct {
	service *QueueService
	require *require.Assertions
	jobName string
	closer  context.CancelFunc
	suite.Suite
}

func TestJobStatusSuite(t *testing.T) {
	suite.Run(t, new(JobStatusSuite))
}

func (s *JobStatusSuite) SetupSuite() {
	s.require = s.Require()
	s.service = NewQueueService()
	ctx, cancel := context.WithCancel(context.Background())
	s.closer = cancel

	s.NoError(s.service.Open(ctx))

	j := job.NewShellJob("echo foo", "")
	s.jobName = j.ID()
	s.NoError(s.service.queue.Put(ctx, j))

	s.NoError(s.service.App().Resolve())
}

func (s *JobStatusSuite) TearDownSuite() {
	s.closer()
}

func (s *JobStatusSuite) TestIncorrectOrInvalidJobNamesReturnExpectedResults() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, name := range []string{"", " ", "123", "DOES-NOT-EXIST", "foo"} {
		resp, err := s.service.getJobStatusResponse(ctx, name)
		s.Error(err)

		s.Equal(err.Error(), resp.Error)
		s.False(resp.Exists)
		s.False(resp.Completed)
		s.Equal(name, resp.ID)
		s.Nil(resp.Job)
	}
}

func (s *JobStatusSuite) TestJobNameReturnsSuccessfulResponse() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	amboy.Wait(ctx, s.service.queue)

	resp, err := s.service.getJobStatusResponse(ctx, s.jobName)
	s.NoError(err)

	s.Equal("", resp.Error)
	s.True(resp.Exists)
	s.True(resp.Completed)
	s.Equal(s.jobName, resp.ID)
}

func (s *JobStatusSuite) TestRequestReturnsErrorInErrorConditions() {
	router, err := s.service.App().Handler()
	s.NoError(err)

	for _, name := range []string{"foo", "bar", "df-df"} {
		w := httptest.NewRecorder()
		req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/v1/job/status/%s", name), nil)

		router.ServeHTTP(w, req)

		s.Equal(400, w.Code)
		jst := jobStatusResponse{}
		err = json.Unmarshal(w.Body.Bytes(), &jst)
		s.NoError(err)

		s.Equal(name, jst.ID)
		s.False(jst.Exists)
		s.False(jst.Completed)
		s.True(jst.Error != "")
	}
}

func (s *JobStatusSuite) TestRequestValidJobStatus() {
	router, err := s.service.App().Handler()
	s.NoError(err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", fmt.Sprintf("http://example.com/v1/job/status/%s", s.jobName), nil)

	router.ServeHTTP(w, req)

	s.Equal(200, w.Code)
	jst := jobStatusResponse{}
	s.NoError(json.Unmarshal(w.Body.Bytes(), &jst))

	s.Equal(s.jobName, jst.ID)
	s.True(jst.Exists)
	s.True(jst.Completed)
	s.Equal("", jst.Error)
}
