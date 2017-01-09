package send

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/bluele/slack"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

const (
	slackClientToken = "GRIP_SLACK_CLIENT_TOKEN"
)

type slackJournal struct {
	channel  string
	hostName string
	fallback *log.Logger
	client   *slack.Slack
	*base
}

// NewSlackLogger constructs a Sender that posts messages to a slack,
// given a slack API token.
//
// You must specify the channel that will receive the messages, and
// the hostname of the current machine, which is added to the logging
// metadata.
func NewSlackLogger(name, token, channel, hostname string, l LevelInfo) (Sender, error) {
	s := &slackJournal{
		hostName: hostname,
		client:   slack.New(token),
		base:     newBase(name),
	}

	if !strings.HasPrefix(channel, "#") {
		s.channel = "#" + channel
	} else {
		s.channel = channel
	}

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	if _, err := s.client.AuthTest(); err != nil {
		return nil, fmt.Errorf("slack authentication error: %v", err)
	}

	s.reset = func() {
		s.fallback = log.New(os.Stdout, strings.Join([]string{"[", s.Name(), "] "}, ""), log.LstdFlags)
	}

	s.reset()

	return s, nil
}

// MakeSlackLogger is equivalent to NewSlackLogger, but constructs a
// Sender reading the slack token from the environment variable
// "GRIP_SLACK_CLIENT_TOKEN".
func MakeSlackLogger(channel string) (Sender, error) {
	token := os.Getenv(slackClientToken)
	if token == "" {
		return nil, fmt.Errorf("environment variable %s not defined, cannot create slack client",
			"foo")
	}

	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("error resolving hostname for slack logger: %v", err)
	}

	return NewSlackLogger("", token, channel, hostname, LevelInfo{level.Trace, level.Trace})
}

func (s *slackJournal) Type() SenderType { return Slack }

func (s *slackJournal) Send(m message.Composer) {
	if !s.level.ShouldLog(m) {
		return
	}

	if fallback, err := s.doSend(m); err != nil {
		s.fallback.Println("slack error:", err.Error())
		s.fallback.Println(fallback)
	}
}

func (s *slackJournal) doSend(m message.Composer) (string, error) {
	msg := m.Resolve()

	s.RLock()
	var channel []byte
	copy(channel, s.channel)
	params := getParams(s.name, s.hostName, m.Priority())
	s.RUnlock()

	if err := s.client.ChatPostMessage(string(channel), msg, params); err != nil {
		return fmt.Sprintf("%s: %s\n", params.Attachments[0].Fallback, msg), err
	}

	return "", nil
}

func getParams(log, host string, p level.Priority) *slack.ChatPostMessageOpt {
	params := slack.ChatPostMessageOpt{
		Attachments: []*slack.Attachment{
			{
				Fallback: fmt.Sprintf("[level=%s, process=%s, host=%s]",
					p, log, host),
				Fields: []*slack.AttachmentField{
					{
						Title: "Host",
						Value: host,
						Short: true,
					},
					{
						Title: "Process",
						Value: log,
						Short: true,
					},
					{
						Title: "Priority",
						Value: p.String(),
						Short: true,
					},
				},
			},
		},
	}

	switch p {
	case level.Emergency, level.Alert, level.Critical:
		params.Attachments[0].Color = "danger"
	case level.Warning, level.Notice:
		params.Attachments[0].Color = "warning"
	default: // includes info/debug
		params.Attachments[0].Color = "good"

	}

	return &params
}
