package send

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"

	hec "github.com/fuyufjh/splunk-hec-go"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

const (
	splunkServerURL   = "GRIP_SPLUNK_SERVER_URL"
	splunkClientToken = "GRIP_SPLUNK_CLIENT_TOKEN"
	splunkChannel     = "GRIP_SPLUNK_CHANNEL"
)

type splunkLogger struct {
	info     SplunkConnectionInfo
	client   splunkClient
	hostname string
	*Base
}

// SplunkConnectionInfo stores all information needed to connect
// to a splunk server to send log messsages.
type SplunkConnectionInfo struct {
	ServerURL string `bson:"url" json:"url" yaml:"url"`
	Token     string `bson:"token" json:"token" yaml:"token"`
	Channel   string `bson:"channel" json:"channel" yaml:"channel"`
}

// GetSplunkConnectionInfo builds a SplunkConnectionInfo structure
// reading default values from the following environment variables:
//
//		GRIP_SPLUNK_SERVER_URL
//		GRIP_SPLUNK_CLIENT_TOKEN
//		GRIP_SPLUNK_CHANNEL
func GetSplunkConnectionInfo() SplunkConnectionInfo {
	return SplunkConnectionInfo{
		ServerURL: os.Getenv(splunkServerURL),
		Token:     os.Getenv(splunkClientToken),
		Channel:   os.Getenv(splunkChannel),
	}
}

// Populated validates a SplunkConnectionInfo, and returns false if
// there is missing data.
func (info SplunkConnectionInfo) Populated() bool {
	return info.ServerURL != "" && info.Token != ""
}

func (s *splunkLogger) Send(m message.Composer) {
	lvl := s.Level()

	if lvl.ShouldLog(m) {
		g, ok := m.(*message.GroupComposer)
		if ok {
			batch := []*hec.Event{}
			for _, c := range g.Messages() {
				if lvl.ShouldLog(c) {
					e := hec.NewEvent(c.Raw())
					e.SetHost(s.hostname)
					batch = append(batch, e)
				}
			}
			if err := s.client.WriteBatch(batch); err != nil {
				s.ErrorHandler(err, m)
			}
			return
		}

		e := hec.NewEvent(m.Raw())
		e.SetHost(s.hostname)
		if err := s.client.WriteEvent(e); err != nil {
			s.ErrorHandler(err, m)
		}
	}
}

// NewSplunkLogger constructs a new Sender implementation that sends
// messages to a Splunk event collector using the credentials specified
// in the SplunkConnectionInfo struct.
func NewSplunkLogger(name string, info SplunkConnectionInfo, l LevelInfo) (Sender, error) {
	s := &splunkLogger{
		info:   info,
		client: &splunkClientImpl{},
		Base:   NewBase(name),
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	s.hostname = hostname

	if err := s.client.Create(info.ServerURL, info.Token, info.Channel); err != nil {
		return nil, err
	}

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	return s, nil
}

// MakeSplunkLogger constructs a new Sender implementation that reads
// the hostname, username, and password from environment variables:
//
//		GRIP_SPLUNK_SERVER_URL
//		GRIP_SPLUNK_CLIENT_TOKEN
//		GRIP_SPLUNK_CLIENT_CHANNEL
func MakeSplunkLogger(name string) (Sender, error) {
	info := GetSplunkConnectionInfo()
	if info.ServerURL == "" {
		return nil, fmt.Errorf("environment variable %s not defined, cannot create splunk client",
			splunkServerURL)
	}
	if info.Token == "" {
		return nil, fmt.Errorf("environment variable %s not defined, cannot create splunk client",
			splunkClientToken)
	}
	return NewSplunkLogger(name, info, LevelInfo{level.Trace, level.Trace})
}

////////////////////////////////////////////////////////////////////////
//
// interface wrapper for the splunk client so that we can mock things out
//
////////////////////////////////////////////////////////////////////////

type splunkClient interface {
	Create(string, string, string) error
	WriteEvent(*hec.Event) error
	WriteBatch([]*hec.Event) error
}

type splunkClientImpl struct {
	hec.HEC
}

func (c *splunkClientImpl) Create(serverURL string, token string, channel string) error {
	c.HEC = hec.NewClient(serverURL, token)
	if channel != "" {
		c.HEC.SetChannel(channel)
	}

	c.HEC.SetKeepAlive(false)
	c.HEC.SetMaxRetry(0)
	c.HEC.SetHTTPClient(&http.Client{
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: 5 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		},
		Timeout: 5 * time.Second,
	})

	return nil
}
