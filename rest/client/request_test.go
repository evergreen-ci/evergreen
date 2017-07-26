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
	// V1 path
	path := s.evergreenREST.getPath("foo", v1)
	s.Equal("url/api/2/foo", path)

	// V2 path
	path = s.evergreenREST.getPath("foo", apiVersion2)
	s.Equal("url/rest/v2/foo", path)
}

func (s *RequestTestSuite) TestValidateRequestInfo() {
	taskData := TaskData{
		ID:     "id",
		Secret: "secret",
	}
	info := requestInfo{
		taskData: &taskData,
		version:  v1,
	}
	err := info.validateRequestInfo()
	s.Error(err)
	validMethods := []method{get, post, put, delete, patch}
	for _, method := range validMethods {
		info.method = method
		err = info.validateRequestInfo()
		s.NoError(err)
	}
	invalidMethods := []method{method("foo"), method("bar")}
	for _, method := range invalidMethods {
		info.method = method
		err = info.validateRequestInfo()
		s.Error(err)
	}
}

func (s *RequestTestSuite) TestSetTaskPathSuffix() {
	info := requestInfo{}
	info.setTaskPathSuffix("foo", "bar")
	s.Equal("task/bar/foo", info.path)
}
