package operations

import (
	"context"
	"os"
	"os/exec"
	"testing"

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
