// +build go1.7

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type CreateJobSuite struct {
	service *QueueService
	require *require.Assertions
	closer  context.CancelFunc
	suite.Suite
}

func TestCreateJobSuite(t *testing.T) {
	suite.Run(t, new(CreateJobSuite))
}

func (s *CreateJobSuite) SetupSuite() {
	s.require = s.Require()
	s.service = NewQueueService()
	ctx, cancel := context.WithCancel(context.Background())
	s.closer = cancel

	s.NoError(s.service.Open(ctx))

	s.NoError(s.service.App().Resolve())
}

func (s *CreateJobSuite) TearDownSuite() {
	s.closer()
}

func (s *CreateJobSuite) TestBaseResponseCreatorHasExpectedValues() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp := s.service.createJobResponseBase(ctx)

	s.Equal(s.service.queue.Stats(ctx).Pending, resp.QueueDepth)
	s.Equal(s.service.getStatus(ctx), resp.Status)
	s.False(resp.Registered)
	s.Equal("", resp.ID)
	s.Equal("", resp.Error)
}

func (s *CreateJobSuite) TestNilJobPayloadResultsInError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	resp, err := s.service.createJob(ctx, &registry.JobInterchange{})
	s.Error(err)
	s.Equal(err.Error(), resp.Error)
	s.False(resp.Registered)
}

func (s *CreateJobSuite) TestAddingAJobThatAlreadyExistsResultsInError() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("true", "")
	payload, err := registry.MakeJobInterchange(j, amboy.JSON)
	s.NoError(err)

	s.NoError(s.service.queue.Put(ctx, j))

	resp, err := s.service.createJob(ctx, payload)
	s.Error(err, fmt.Sprintf("%+v", resp))

	s.Equal(err.Error(), resp.Error)
	s.Equal(j.ID(), resp.ID)
}

func (s *CreateJobSuite) TestAddingJobSuccessfuly() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	j := job.NewShellJob("true", "")

	payload, err := registry.MakeJobInterchange(j, amboy.JSON)
	s.NoError(err)

	resp, err := s.service.createJob(ctx, payload)
	s.NoError(err)

	s.Equal(j.ID(), resp.ID)
	s.True(resp.Registered)
	s.Equal("", resp.Error)
}

func (s *CreateJobSuite) TestRequestWithNilPayload() {
	router, err := s.service.App().Handler()
	s.NoError(err)

	rb, err := json.Marshal(`{}`)
	s.NoError(err)
	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://example.com/v1/job/create", bytes.NewBuffer(rb))

	router.ServeHTTP(w, req)
	s.Equal(400, w.Code)

	resp := createResponse{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	s.NoError(err)

	s.True(resp.Error != "")
	s.False(resp.Registered)
}

func (s *CreateJobSuite) TestRequestToAddJobThatAlreadyExists() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router, err := s.service.App().Handler()
	s.NoError(err)

	payload, err := registry.MakeJobInterchange(job.NewShellJob("true", ""), amboy.JSON)
	s.NoError(err)

	rb, err := json.Marshal(payload)
	s.NoError(err)

	j, err := payload.Resolve(amboy.JSON)
	s.NoError(err)

	s.NoError(s.service.queue.Put(ctx, j))

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://example.com/v1/job/create", bytes.NewBuffer(rb))
	router.ServeHTTP(w, req)
	s.Equal(400, w.Code)

	resp := createResponse{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	s.NoError(err)

	s.True(resp.Error != "")
	s.Equal(j.ID(), resp.ID)
	s.False(resp.Registered)
}

func (s *CreateJobSuite) TestRequestToAddNewJobRegistersJob() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router, err := s.service.App().Handler()
	s.NoError(err)

	startingTotal := s.service.queue.Stats(ctx).Total
	j := job.NewShellJob("true", "")
	payload, err := registry.MakeJobInterchange(j, amboy.JSON)
	s.NoError(err)

	rb, err := json.Marshal(payload)
	s.NoError(err)

	w := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "http://example.com/v1/job/create", bytes.NewBuffer(rb))

	router.ServeHTTP(w, req)
	s.Equal(200, w.Code)

	resp := createResponse{}
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	s.NoError(err)

	s.True(resp.Error == "")
	s.True(resp.Registered)
	s.Equal(j.ID(), resp.ID)
	s.Equal(s.service.queue.Stats(ctx).Total, startingTotal+1)
}
