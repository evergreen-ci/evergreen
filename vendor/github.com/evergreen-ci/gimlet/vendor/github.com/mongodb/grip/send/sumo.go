package send

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"os"

	"github.com/mongodb/grip/message"
	sumo "github.com/nutmegdevelopment/sumologic/upload"
)

type sumoLogger struct {
	endpoint string
	client   sumoClient
	*Base
}

const sumoEndpointEnvVar = "GRIP_SUMO_ENDPOINT"

// MakeSumo constructs a Sumo logging backend that reads the private
// URL endpoint from the GRIP_SUMO_ENDPOINT environment variable.
//
// The instance is otherwise unconquered. Call SetName or inject it
// into a Journaler instance using SetSender before using.
//
// The logger uses the JSON formatter by default.
func MakeSumo() (Sender, error) {
	endpoint := os.Getenv(sumoEndpointEnvVar)

	s, err := NewSumo("", endpoint)
	if err != nil {
		return nil, err
	}

	return s, nil
}

// NewSumo constructs a Sumo logging backend that sends messages to the
// private URL endpoint of an HTTP source.
//
// The logger uses the JSON formatter by default.
func NewSumo(name, endpoint string) (Sender, error) {
	s := &sumoLogger{
		endpoint: endpoint,
		client:   &sumoClientImpl{},
		Base:     NewBase(name),
	}

	s.client.Create(s.endpoint)
	if _, err := url.ParseRequestURI(s.endpoint); err != nil {
		return nil, fmt.Errorf("cannot connect to url '%s': %s", s.endpoint, err)
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	if err := s.SetFormatter(MakeJSONFormatter()); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
	}

	return s, nil
}

func (s *sumoLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		text, err := s.formatter(m)
		if err != nil {
			s.ErrorHandler()(err, m)
			return
		}

		buf := []byte(text)
		if err := s.client.Send(buf, s.name); err != nil {
			s.ErrorHandler()(err, m)
		}
	}
}

func (s *sumoLogger) Flush(_ context.Context) error { return nil }

////////////////////////////////////////////////////////////////////////
//
// interface to wrap sumologic client interaction
//
////////////////////////////////////////////////////////////////////////

type sumoClient interface {
	Create(string)
	Send([]byte, string) error
}

type sumoClientImpl struct {
	sumo.Uploader
}

func (c *sumoClientImpl) Create(endpoint string) {
	c.Uploader = sumo.NewUploader(endpoint)
}
