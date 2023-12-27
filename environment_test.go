package evergreen

import (
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"strings"
	"testing"

	"github.com/evergreen-ci/pail"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	if skip, _ := strconv.ParseBool(os.Getenv("AUTH_ENABLED")); skip {
		// The DB auth test cannot initialize the environment due to
		// requiring DB auth credentials.
		return
	}

	if GetEnvironment() == nil {
		ctx := context.Background()

		path := testConfigFile()
		env, err := NewEnvironment(ctx, path, nil)
		grip.EmergencyFatal(message.WrapError(err, message.Fields{
			"message": "could not initialize test environment",
			"path":    path,
		}))

		SetEnvironment(env)
	}
}

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
	s.env = &envState{
		senders: map[SenderKey]send.Sender{},
	}
}

func (s *EnvironmentSuite) TestInitDB() {
	db := &DBSettings{
		Url: "mongodb://localhost:27017",
		DB:  "mci_test",
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	localEnv := s.env
	err := localEnv.initDB(ctx, *db)
	s.NoError(err)
	_, err = localEnv.client.ListDatabases(ctx, bson.M{})
	s.NoError(err)
}

func (s *EnvironmentSuite) TestLoadingConfig() {
	s.shouldSkip()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	originalEnv := GetEnvironment()
	// first test loading config from a file
	env, err := NewEnvironment(ctx, s.path, nil)
	SetEnvironment(env)
	defer func() {
		SetEnvironment(originalEnv)
	}()

	s.Require().NoError(err)
	s.env = env.(*envState)
	s.Equal("http://localhost:9090", s.env.Settings().ApiUrl)
	// persist to db
	s.NoError(s.env.SaveConfig(ctx))

	// then test loading it from the db
	s.env.settings = nil
	settings, err := NewSettings(s.path)
	s.NoError(err)
	db := settings.Database

	env, err = NewEnvironment(ctx, "", &db)
	s.Require().NoError(err)
	SetEnvironment(env)

	s.env = env.(*envState)
	s.Equal(db, s.env.settings.Database)
	s.Equal("http://localhost:9090", s.env.Settings().ApiUrl)
}

func (s *EnvironmentSuite) TestConfigErrorsIfCannotValidateConfig() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.env.settings = &Settings{}
	err := s.env.initSettings(ctx, "")
	s.Error(err)
	s.Contains(err.Error(), "validating settings")
}

func (s *EnvironmentSuite) TestGetClientConfig() {
	tmpDir := s.T().TempDir()
	BuildRevision = "abcdef"
	prefix := "evergreen/clients/evergreen"
	platforms := []string{
		ArchDarwinAmd64,
		ArchDarwinArm64,
		ArchLinuxPpc64le,
		ArchLinuxS390x,
		ArchLinuxArm64,
		ArchLinuxAmd64,
		ArchWindowsAmd64,
	}
	for _, platform := range platforms {
		basePath := path.Join(tmpDir, prefix, BuildRevision, platform)
		s.Require().NoError(os.MkdirAll(basePath, os.ModeDir|os.ModePerm))
		executable := "evergreen"
		if strings.Contains(platform, "windows") {
			executable += ".exe"
		}
		_, err := os.Create(path.Join(basePath, executable))
		s.Require().NoError(err)
	}

	bucket, err := pail.NewLocalBucket(pail.LocalOptions{
		Path: tmpDir,
	})
	s.Require().NoError(err)

	e := envState{settings: &Settings{}}
	e.settings.Providers.AWS.BinaryClient = BinaryClientS3Config{
		S3Credentials: S3Credentials{Bucket: tmpDir},
		Prefix:        prefix,
	}

	binaries, err := e.listClientBinaries(context.Background(), bucket)
	s.NoError(err)
	s.Len(binaries, len(platforms))
	for _, binary := range binaries {
		s.NotEmpty(binary.URL)
		s.NotEmpty(binary.DisplayName)
		s.Equal(ValidArchDisplayNames[fmt.Sprintf("%s_%s", binary.OS, binary.Arch)], binary.DisplayName)
	}
}
