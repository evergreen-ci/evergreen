package send

import (
	"os"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type XMPPSuite struct {
	info XMPPConnectionInfo
	suite.Suite
}

func TestXMPPSuite(t *testing.T) {
	suite.Run(t, new(XMPPSuite))
}

func (s *XMPPSuite) SetupSuite() {}

func (s *XMPPSuite) SetupTest() {
	s.info = XMPPConnectionInfo{
		client: &xmppClientMock{},
	}
}

func (s *XMPPSuite) TestEnvironmentVariableReader() {
	hostVal := "hostName"
	userVal := "userName"
	passVal := "passName"

	defer os.Setenv(xmppHostEnvVar, os.Getenv(xmppHostEnvVar))
	defer os.Setenv(xmppUsernameEnvVar, os.Getenv(xmppUsernameEnvVar))
	defer os.Setenv(xmppPasswordEnvVar, os.Getenv(xmppPasswordEnvVar))

	s.NoError(os.Setenv(xmppHostEnvVar, hostVal))
	s.NoError(os.Setenv(xmppUsernameEnvVar, userVal))
	s.NoError(os.Setenv(xmppPasswordEnvVar, passVal))

	info := GetXMPPConnectionInfo()

	s.Equal(hostVal, info.Hostname)
	s.Equal(userVal, info.Username)
	s.Equal(passVal, info.Password)
}

func (s *XMPPSuite) TestNewConstructor() {
	sender, err := NewXMPPLogger("name", "target", s.info, LevelInfo{level.Debug, level.Info})
	s.NoError(err)
	s.NotNil(sender)
}

func (s *XMPPSuite) TestNewConstructorFailsWhenClientCreateFails() {
	s.info.client = &xmppClientMock{failCreate: true}

	sender, err := NewXMPPLogger("name", "target", s.info, LevelInfo{level.Debug, level.Info})
	s.Error(err)
	s.Nil(sender)
}

func (s *XMPPSuite) TestCloseMethod() {
	sender, err := NewXMPPLogger("name", "target", s.info, LevelInfo{level.Debug, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.info.client.(*xmppClientMock)
	s.True(ok)
	s.Equal(0, mock.numCloses)
	s.NoError(sender.Close())
	s.Equal(1, mock.numCloses)
}

func (s *XMPPSuite) TestAutoConstructorErrorsWithoutValidEnvVar() {
	sender, err := MakeXMPP("target")
	s.Error(err)
	s.Nil(sender)

	sender, err = NewXMPP("target", "name", LevelInfo{level.Debug, level.Info})
	s.Error(err)
	s.Nil(sender)
}

func (s *XMPPSuite) TestSendMethod() {
	sender, err := NewXMPPLogger("name", "target", s.info, LevelInfo{level.Debug, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.info.client.(*xmppClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Debug, "hello")
	sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "")
	sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numSent, 1)
}

func (s *XMPPSuite) TestSendMethodWithError() {
	sender, err := NewXMPPLogger("name", "target", s.info, LevelInfo{level.Debug, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.info.client.(*xmppClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)
	s.False(mock.failSend)

	m := message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numSent, 1)

	mock.failSend = true
	sender.Send(m)
	s.Equal(mock.numSent, 1)
}
