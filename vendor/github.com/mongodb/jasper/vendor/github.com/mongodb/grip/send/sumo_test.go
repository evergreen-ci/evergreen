package send

import (
	"os"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type SumoSuite struct {
	endpoint string
	client   sumoClient
	sender   sumoLogger
	suite.Suite
}

func TestSumoSuite(t *testing.T) {
	suite.Run(t, new(SumoSuite))
}

func (s *SumoSuite) SetupSuite() {}

func (s *SumoSuite) SetupTest() {
	s.endpoint = "http://endpointVal"
	s.client = &sumoClientMock{}

	s.sender = sumoLogger{
		endpoint: s.endpoint,
		client:   s.client,
		Base:     NewBase("name"),
	}

	s.NoError(s.sender.SetFormatter(MakeJSONFormatter()))
	s.NoError(s.sender.SetLevel(LevelInfo{level.Debug, level.Info}))
}

func (s *SumoSuite) TestConstructorEnvVar() {
	defer os.Setenv(sumoEndpointEnvVar, os.Getenv(sumoEndpointEnvVar))

	s.NoError(os.Setenv(sumoEndpointEnvVar, s.endpoint))

	sender, err := MakeSumo()
	s.NoError(err)
	s.NotNil(sender)

	s.NoError(os.Unsetenv(sumoEndpointEnvVar))

	sender, err = MakeSumo()
	s.Error(err)
	s.Nil(sender)
}

func (s *SumoSuite) TestConstructorEndpointString() {
	sender, err := NewSumo("name", s.endpoint)
	s.NoError(err)
	s.NotNil(sender)

	sender, err = NewSumo("name", "invalidEndpoint")
	s.Error(err)
	s.Nil(sender)
}

func (s *SumoSuite) TestSendMethod() {
	mock, ok := s.client.(*sumoClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Debug, "hello")
	s.sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "")
	s.sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "world")
	s.sender.Send(m)
	s.Equal(mock.numSent, 1)
}

func (s *SumoSuite) TestSendMethodWithError() {
	mock, ok := s.client.(*sumoClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)
	s.False(mock.failSend)

	m := message.NewDefaultMessage(level.Alert, "world")
	s.sender.Send(m)
	s.Equal(mock.numSent, 1)

	mock.failSend = true
	s.sender.Send(m)
	s.Equal(mock.numSent, 1)
}
