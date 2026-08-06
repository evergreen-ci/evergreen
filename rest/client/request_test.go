package client

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		maxAttempts:  10,
		timeoutStart: time.Second * 2,
		timeoutMax:   time.Minute * 10,
		serverURL:    "url",
	}
}

func TestRateLimitMessage(t *testing.T) {
	// The rate limit header names aren't in canonical MIME form, so they must be
	// set with Set rather than assigned directly into the header map.
	newHeader := func(kv map[string]string) http.Header {
		h := http.Header{}
		for k, v := range kv {
			h.Set(k, v)
		}
		return h
	}

	for testName, testCase := range map[string]struct {
		header           http.Header
		expectedContains []string
		expectedOmits    []string
	}{
		"AllHeadersShouldBeIncluded": {
			header: newHeader(map[string]string{
				evergreen.RateLimitLimitHeader: "5000",
				evergreen.RetryAfterHeader:     "7",
			}),
			expectedContains: []string{"5000", "7", "/rest/v2/users/{user_id}/rate_limit"},
		},
		"MissingRetryAfterShouldOnlyReportLimit": {
			header: newHeader(map[string]string{
				evergreen.RateLimitLimitHeader: "5000",
			}),
			expectedContains: []string{"5000"},
			expectedOmits:    []string{"retry in"},
		},
		"MissingLimitShouldOnlyReportRetryAfter": {
			header: newHeader(map[string]string{
				evergreen.RetryAfterHeader: "7",
			}),
			expectedContains: []string{"7"},
			expectedOmits:    []string{"refills"},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			msg := RateLimitMessage(testCase.header)
			assert.Contains(t, msg, "rate limit exceeded")
			for _, contains := range testCase.expectedContains {
				assert.Contains(t, msg, contains)
			}
			for _, omits := range testCase.expectedOmits {
				assert.NotContains(t, msg, omits)
			}
		})
	}
}

func TestRequestTooManyRequestsShouldErrorWithRateLimitInfo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set(evergreen.RateLimitLimitHeader, "5000")
		rw.Header().Set(evergreen.RetryAfterHeader, "7")
		rw.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)

	c := &communicatorImpl{serverURL: srv.URL, httpClient: srv.Client()}
	resp, err := c.request(t.Context(), requestInfo{method: http.MethodGet, path: "path"}, nil)
	require.Error(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, http.StatusTooManyRequests, resp.StatusCode)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Contains(t, err.Error(), "5000")
	assert.Contains(t, err.Error(), "7")
}

func (s *RequestTestSuite) TestNewRequest() {
	r, err := s.evergreenREST.newRequest(http.MethodGet, "path", nil)
	s.NoError(err)
	s.Equal(evergreen.ContentTypeValue, r.Header.Get(evergreen.ContentTypeHeader))
}

func (s *RequestTestSuite) TestGetPathReturnsCorrectPath() {
	path := s.evergreenREST.getPath("foo")
	s.Equal("url/rest/v2/foo", path)
}

func (s *RequestTestSuite) TestValidateRequestInfo() {
	info := requestInfo{}
	err := info.validateRequestInfo()
	s.Error(err)
	validMethods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range validMethods {
		info.method = method
		err = info.validateRequestInfo()
		s.NoError(err)
	}
	invalidMethods := []string{"foo", "bar"}
	for _, method := range invalidMethods {
		info.method = method
		err = info.validateRequestInfo()
		s.Error(err)
	}
}
