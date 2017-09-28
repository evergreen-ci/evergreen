package cli

import (
	"io/ioutil"
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
const testApiServer = "testurl"

func TestCliHttpTestSuite(t *testing.T) {
	suite.Run(t, new(CliHttpTestSuite))
}

// sets up the global settings file
func (s *CliHttpTestSuite) SetupSuite() {
	fileContents := "user: \"" + testUserName + "\""
	fileContents += "\napi_key: \"" + testApiKey + "\""
	fileContents += "\napi_server_host: \"" + testApiServer + "\""
	err := ioutil.WriteFile(testFileName, []byte(fileContents), 0644)
	s.NoError(err)
}

// tests to make sure that an API V2 client can be created with the right settings
func (s *CliHttpTestSuite) TestV2Client() {
	opts := &Options{
		ConfFile: testFileName,
	}

	client, settings, err := getAPIV2Client(opts)
	s.NoError(err)
	s.NotNil(client)
	s.NotNil(settings)
	s.Equal(testApiKey, settings.APIKey)
	s.Equal(testApiServer, settings.APIServerHost)
	s.Equal(testUserName, settings.User)
}

// cleans up the test settings file
func (s *CliHttpTestSuite) TearDownSuite() {
	cmd := exec.Command("rm", "-f", testFileName)
	err := cmd.Run()
	s.NoError(err)
}
