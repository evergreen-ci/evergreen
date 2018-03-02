package evergreen

import (
	"os"
	"path/filepath"
	"strings"
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

	// first test loading config from a file
	s.NoError(s.env.Configure(ctx, s.path, nil))
	s.Error(s.env.Configure(ctx, s.path, nil))

	// then test loading it from the db
	s.env.settings = nil
	settings, err := NewSettings(s.path)
	s.NoError(err)
	db := settings.Database
	s.NoError(s.env.Configure(ctx, "", &db))
	s.Equal(db, s.env.settings.Database)
}

func (s *EnvironmentSuite) TestConfigErrorsIfCannotValidateConfig() {
	s.env.settings = &Settings{}
	err := s.env.initSettings("")
	s.Error(err)
	s.Contains(err.Error(), "validating settings")
}

func (s *EnvironmentSuite) TestGetClientConfig() {
	root := filepath.Join(FindEvergreenHome(), ClientDirectory)
	if err := os.Mkdir(root, os.ModeDir|os.ModePerm); err != nil {
		s.True(os.IsExist(err))
	}

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
		defer func() { //nolint: evg
			s.NoError(os.Remove(file))
			s.NoError(os.Remove(path))
		}()
	}

	client, err := getClientConfig("https://example.com")
	s.NoError(err)
	s.Require().NotNil(client)

	s.Equal(ClientVersion, client.LatestRevision)

	cb := []ClientBinary{}
	for _, b := range client.ClientBinaries {
		if strings.Contains(b.URL, "_obviouslynotherealone/") {
			cb = append(cb, b)
		}
	}

	s.Require().Len(cb, 2)
	s.Equal("amd64", cb[0].Arch)
	s.Equal("darwin", cb[0].OS)
	s.Equal("https://example.com/clients/darwin_amd64_obviouslynotherealone/evergreen", cb[0].URL)
	s.Equal("z80", cb[1].Arch)
	s.Equal("linux", cb[1].OS)
	s.Equal("https://example.com/clients/linux_z80_obviouslynotherealone/evergreen", cb[1].URL)
}
