package client

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/suite"
)

type RequestTestSuite struct {
	suite.Suite
	evergreenREST *communicatorImpl
}

func TestRequestTestSuite(t *testing.T) {
	suite.Run(t, new(RequestTestSuite))
}

func (s *RequestTestSuite) SetupTest() {
	s.evergreenREST = &communicatorImpl{
		hostID:       "hostID",
		hostSecret:   "hostSecret",
		maxAttempts:  10,
		timeoutStart: time.Second * 2,
		timeoutMax:   time.Minute * 10,
		serverURL:    "url",
	}
}

func (s *RequestTestSuite) TestNewRequest() {
	r, err := s.evergreenREST.newRequest("method", "path", "taskSecret", v1, nil)
	s.NoError(err)
	s.Equal("taskSecret", r.Header.Get(evergreen.TaskSecretHeader))
	s.Equal(s.evergreenREST.hostID, r.Header.Get(evergreen.HostHeader))
	s.Equal(s.evergreenREST.hostSecret, r.Header.Get(evergreen.HostSecretHeader))
	s.Equal(evergreen.ContentTypeValue, r.Header.Get(evergreen.ContentTypeHeader))
}

func (s *RequestTestSuite) TestGetPathReturnsCorrectPath() {
	path := s.evergreenREST.getPath("foo", v1)
	s.Equal("url/api/2/foo", path)
}

func (s *RequestTestSuite) TestGetPathSuffix() {
	s.Equal("task/foo/bar", s.evergreenREST.getTaskPathSuffix("bar", "foo"))
}
