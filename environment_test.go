package evergreen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type EnvironmentSuite struct {
	path string
	env  *envState
	suite.Suite
}

func TestEnvironmentSuite(t *testing.T) {
	assert.Implements(t, (*Environment)(nil), &envState{})

	suite.Run(t, new(EnvironmentSuite))
}

func (s *EnvironmentSuite) SetupSuite() {
	s.path = os.Getenv("SETTINGS_OVERRIDE")
}

func (s *EnvironmentSuite) shouldSkip() {
	if s.path == "" {
		s.T().Skip("settings not configured")
	}
}

func (s *EnvironmentSuite) SetupTest() {
	s.env = &envState{}
}

func (s *EnvironmentSuite) TestLoadingConfig() {
	s.shouldSkip()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.NoError(s.env.Configure(ctx, s.path))
	s.Error(s.env.Configure(ctx, s.path))
}

func (s *EnvironmentSuite) TestLoadingConfigErrorsIfFileDoesNotExist() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Error(s.env.Configure(ctx, ""))

	s.Error(s.env.Configure(ctx, s.path+"-DOES-NOT-EXIST"))
}

func (s *EnvironmentSuite) TestConfigErrorsIfCannotValidateConfig() {
	s.env.settings = &Settings{}
	err := s.env.initSettings("")
	s.Error(err)
	s.Contains(err.Error(), "validating settings")
}

func (s *EnvironmentSuite) TestGetClientConfig() {
	root := filepath.Join(FindEvergreenHome(), ClientDirectory)
	folders := []string{
		"darwin_amd64_obviouslynotherealone",
		"linux_z80_obviouslynotherealone",
	}
	for _, folder := range folders {
		path := root + "/" + folder
		s.NoError(os.Mkdir(path, os.ModeDir|os.ModePerm))

		file := path + "/evergreen"
		_, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		s.NoError(err)
		defer func() {
			os.Remove(file)
			os.Remove(path)
		}()
	}

	client, err := getClientConfig("https://example.com")
	s.NoError(err)
	s.Require().NotNil(client)

	s.Equal(ClientVersion, client.LatestRevision)
	s.Require().Len(client.ClientBinaries, 2)
	s.Equal("amd64", client.ClientBinaries[0].Arch)
	s.Equal("darwin", client.ClientBinaries[0].OS)
	s.Equal("https://example.com/clients/darwin_amd64_obviouslynotherealone/evergreen", client.ClientBinaries[0].URL)
	s.Equal("z80", client.ClientBinaries[1].Arch)
	s.Equal("linux", client.ClientBinaries[1].OS)
	s.Equal("https://example.com/clients/linux_z80_obviouslynotherealone/evergreen", client.ClientBinaries[1].URL)
}
