package send

import (
	"fmt"
	"log"
	"os"
	"strings"

	xmpp "github.com/mattn/go-xmpp"
	"github.com/tychoish/grip/message"
)

type xmppLogger struct {
	target string
	client *xmpp.Client
	*base
}

// XMPPConnectionInfo stores all information needed to connect to an
// XMPP (jabber) server to send log messages.
type XMPPConnectionInfo struct {
	Hostname string
	Username string
	Password string
}

const (
	xmppHostEnvVar     = "GRIP_XMMP_HOSTNAME"
	xmppUsernameEnvVar = "GRIP_XMMP_USERNAME"
	xmppPasswordEnvVar = "GRIP_XMMP_PASSWORD"
)

// GetXMPPConnectionInfo builds an XMPPConnectionInfo structure
// reading default values from the following environment variables:
//
//    GRIP_XMPP_HOSTNAME
//    GRIP_XMPP_USERNAME
//    GRIP_XMPP_PASSWORD
func GetXMPPConnectionInfo() XMPPConnectionInfo {
	return XMPPConnectionInfo{
		Hostname: os.Getenv(xmppHostEnvVar),
		Username: os.Getenv(xmppUsernameEnvVar),
		Password: os.Getenv(xmppPasswordEnvVar),
	}
}

// NewXMPPLogger constructs a new Sender implementation that sends
// messages to an XMPP user, "target", using the credentials specified in
// the XMPPConnectionInfo struct. The constructor will attempt to exablish
// a connection to the server via SSL, falling back automatically to an
// unencrypted connection if the the first attempt fails.
func NewXMPPLogger(name, target string, info XMPPConnectionInfo, l LevelInfo) (Sender, error) {
	s, err := constructXMPPLogger(target, info)
	if err != nil {
		return nil, err
	}

	return setup(s, name, l)
}

// MakeXMPP constructs an XMPP logging backend that reads the
// hostname, username, and password from environment variables:
//
//    - GRIP_XMPP_HOSTNAME
//    - GRIP_XMPP_USERNAME
//    - GRIP_XMPP_PASSWORD
//
// The instance is otherwise unconquered. Call SetName or inject it
// into a Journaler instance using SetSender before using.
func MakeXMPP(target string) (Sender, error) {
	info := GetXMPPConnectionInfo()

	s, err := constructXMPPLogger(target, info)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// NewXMPP constructs an XMPP logging backend that reads the
// hostname, username, and password from environment variables:
//
//    - GRIP_XMPP_HOSTNAME
//    - GRIP_XMPP_USERNAME
//    - GRIP_XMPP_PASSWORD
//
// Otherwise, the semantics of NewXMPPDefault are the same as NewXMPPLogger.
func NewXMPP(name, target string, l LevelInfo) (Sender, error) {
	info := GetXMPPConnectionInfo()

	return NewXMPPLogger(name, target, info, l)
}

func constructXMPPLogger(target string, info XMPPConnectionInfo) (Sender, error) {
	s := &xmppLogger{
		base:   newBase(""),
		target: target,
	}

	client, err := xmpp.NewClient(info.Hostname, info.Username, info.Password, false)
	if err != nil {
		errs := []string{err.Error()}
		client, err = xmpp.NewClientNoTLS(info.Hostname, info.Username, info.Password, false)
		if err != nil {
			errs = append(errs, err.Error())
			return nil, fmt.Errorf("cannot connect to server '%s', as '%s': %s",
				info.Hostname, info.Username, strings.Join(errs, "; "))
		}
	}
	s.client = client

	s.closer = func() error {
		return client.Close()
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s]", s.Name()))
	}

	return s, nil
}

func (s *xmppLogger) Send(m message.Composer) {
	if s.level.ShouldLog(m) {
		c := xmpp.Chat{
			Remote: s.target,
			Type:   "chat",
			Text:   fmt.Sprintf("[%s] (p=%s)  %s", s.Name(), m.Priority(), m.String()),
		}

		if _, err := s.client.Send(c); err != nil {
			s.errHandler(err, m)
		}
	}
}
