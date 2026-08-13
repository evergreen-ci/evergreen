package operations

import (
	"context"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type CliHttpTestSuite struct {
	suite.Suite
}

const testFileName = ".evergreen_test.yml"
const testUserName = "testUser"
const testApiKey = "1234567890abcdef"
const testApiServer = "http://example.invalid"

func TestCliHttpTestSuite(t *testing.T) {
	suite.Run(t, new(CliHttpTestSuite))
}

func TestNewAPIErrorTooManyRequestsShouldReportRateLimitInfo(t *testing.T) {
	header := http.Header{}
	header.Set(evergreen.RateLimitLimitHeader, "5000")
	header.Set(evergreen.RetryAfterHeader, "7")
	resp := &http.Response{
		StatusCode: http.StatusTooManyRequests,
		Status:     "429 Too Many Requests",
		Header:     header,
		Body:       io.NopCloser(strings.NewReader(`{"status":429,"message":"rate limit exceeded"}`)),
	}

	err := NewAPIError(resp)
	assert.Contains(t, err.Error(), "rate limit exceeded")
	assert.Contains(t, err.Error(), "5000")
	assert.Contains(t, err.Error(), "7")
}

func TestNewAPIErrorNonRateLimitStatusShouldReportResponseBody(t *testing.T) {
	resp := &http.Response{
		StatusCode: http.StatusNotFound,
		Status:     "404 Not Found",
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader("no such project")),
	}

	err := NewAPIError(resp)
	assert.Contains(t, err.Error(), "no such project")
}

// sets up the global settings file
func (s *CliHttpTestSuite) SetupSuite() {
	fileContents := "user: \"" + testUserName + "\""
	fileContents += "\napi_key: \"" + testApiKey + "\""
	fileContents += "\napi_server_host: \"" + testApiServer + "\""
	err := os.WriteFile(testFileName, []byte(fileContents), 0644)
	s.NoError(err)
}

// tests to make sure that an API V2 client can be created with the right settings
func (s *CliHttpTestSuite) TestV2Client() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	settings, err := NewClientSettings(testFileName)
	s.Require().NoError(err)
	client, err := settings.setupRestCommunicator(ctx, true)
	s.Require().NoError(err)
	defer client.Close()
	if s.NoError(err) {
		s.NotNil(client)
		s.NotNil(settings)
		s.Equal(testApiKey, settings.APIKey)
		s.Equal(testApiServer, settings.APIServerHost)
		s.Equal(testUserName, settings.User)
	}
}

// cleans up the test settings file
func (s *CliHttpTestSuite) TearDownSuite() {
	cmd := exec.Command("rm", "-f", testFileName)
	err := cmd.Run()
	s.NoError(err)
}
