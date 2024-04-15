package evergreen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.opentelemetry.io/otel/trace/noop"
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
		env, err := NewEnvironment(ctx, path, "", nil, noop.NewTracerProvider())
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
	err := localEnv.initDB(ctx, *db, noop.NewTracerProvider().Tracer(""))
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
	env, err := NewEnvironment(ctx, s.path, "", nil, noop.NewTracerProvider())
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

	env, err = NewEnvironment(ctx, "", "", &db, noop.NewTracerProvider())
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
	err := s.env.initSettings(ctx, "", noop.NewTracerProvider().Tracer(""))
	s.Error(err)
	s.Contains(err.Error(), "validating settings")
}

func (s *EnvironmentSuite) TestInitSenders() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.env.settings = &Settings{
		Notify: NotifyConfig{
			SES: SESConfig{
				SenderAddress: "sender_address",
			},
		},
	}

	s.Require().NoError(s.env.initThirdPartySenders(ctx, noop.NewTracerProvider().Tracer("")))

	s.Require().NotEmpty(s.env.senders, "should have set up at least one sender")
	for _, sender := range s.env.senders {
		s.NotZero(sender.ErrorHandler(), "fallback error handler should be set")
	}
}

func (s *EnvironmentSuite) TestPopulateLocalClientConfig() {
	root := filepath.Join(FindEvergreenHome(), ClientDirectory)
	if err := os.Mkdir(root, os.ModeDir|os.ModePerm); err != nil {
		s.True(os.IsExist(err))
	}

	folders := []string{
		"darwin_amd64_obviouslynottherealone",
		"linux_z80_obviouslynottherealone",
	}
	for _, folder := range folders {
		path := root + "/" + folder
		s.NoError(os.Mkdir(path, os.ModeDir|os.ModePerm))

		file := path + "/evergreen"
		_, err := os.OpenFile(file, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
		s.NoError(err)
		defer func() { //nolint: evg-lint
			_ = os.Remove(file)
			_ = os.Remove(path)
		}()
	}

	e := envState{
		settings: &Settings{
			HostInit: HostInitConfig{S3BaseURL: "https://another-example.com"},
			Ui:       UIConfig{Url: "https://example.com"},
		},
	}
	s.Require().NoError(e.populateLocalClientConfig())
	s.Require().NotNil(e.clientConfig)

	s.Equal(ClientVersion, e.clientConfig.LatestRevision)

	cb := []ClientBinary{}
	for _, b := range e.clientConfig.ClientBinaries {
		if strings.Contains(b.URL, "_obviouslynottherealone/") {
			cb = append(cb, b)
		}
	}

	s.Require().Len(cb, 2)
	s.Equal("amd64", cb[0].Arch)
	s.Equal("darwin", cb[0].OS)
	s.Equal("https://example.com/clients/darwin_amd64_obviouslynottherealone/evergreen", cb[0].URL)
	s.Equal("z80", cb[1].Arch)
	s.Equal("linux", cb[1].OS)
	s.Equal("https://example.com/clients/linux_z80_obviouslynottherealone/evergreen", cb[1].URL)

	s.Require().NoError(e.populateLocalClientConfig())
	s.Require().NotNil(e.clientConfig)

	s.Equal(ClientVersion, e.clientConfig.LatestRevision)

	var binaries []ClientBinary
	for _, b := range e.clientConfig.ClientBinaries {
		if strings.Contains(b.URL, "_obviouslynottherealone/") {
			binaries = append(binaries, b)
		}
	}

	s.Require().Len(binaries, 2)
	s.Equal("amd64", binaries[0].Arch)
	s.Equal("darwin", binaries[0].OS)
	s.Equal("https://example.com/clients/darwin_amd64_obviouslynottherealone/evergreen", binaries[0].URL)
	s.Equal("z80", binaries[1].Arch)
	s.Equal("linux", binaries[1].OS)
	s.Equal("https://example.com/clients/linux_z80_obviouslynottherealone/evergreen", binaries[1].URL)

	var s3Binaries []ClientBinary
	for _, b := range e.clientConfig.S3ClientBinaries {
		if strings.Contains(b.URL, "_obviouslynottherealone/") {
			s3Binaries = append(s3Binaries, b)
		}
	}

	s.Require().Len(s3Binaries, 2)
	s.Equal("amd64", s3Binaries[0].Arch)
	s.Equal("darwin", s3Binaries[0].OS)
	s.Equal(fmt.Sprintf("https://another-example.com/%s/darwin_amd64_obviouslynottherealone/evergreen", BuildRevision), s3Binaries[0].URL)
	s.Equal("z80", s3Binaries[1].Arch)
	s.Equal("linux", s3Binaries[1].OS)
	s.Equal(fmt.Sprintf("https://another-example.com/%s/linux_z80_obviouslynottherealone/evergreen", BuildRevision), s3Binaries[1].URL)
}
