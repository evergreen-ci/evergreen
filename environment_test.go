package evergreen

import (
	"context"
	"os"
	"strconv"
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
		env, err := NewEnvironment(ctx, path, "", "", nil, noop.NewTracerProvider())
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
	s.path = os.Getenv(SettingsOverride)
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
	env, err := NewEnvironment(ctx, s.path, "", "", nil, noop.NewTracerProvider())
	SetEnvironment(env)
	defer func() {
		SetEnvironment(originalEnv)
	}()

	s.Require().NoError(err)
	s.env = env.(*envState)
	s.Equal("http://localhost:9090", s.env.Settings().Api.URL)
	// persist to db
	s.NoError(s.env.SaveConfig(ctx))

	// then test loading it from the db
	s.env.settings = nil
	settings, err := NewSettings(s.path)
	s.NoError(err)
	db := settings.Database

	env, err = NewEnvironment(ctx, "", "", "", &db, noop.NewTracerProvider())
	s.Require().NoError(err)
	SetEnvironment(env)

	s.env = env.(*envState)
	s.Equal(db, s.env.settings.Database)
	s.Equal("http://localhost:9090", s.env.Settings().Api.URL)
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
