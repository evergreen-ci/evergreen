package send

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/bluele/slack"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

const (
	slackClientToken = "GRIP_SLACK_CLIENT_TOKEN"
)

type slackJournal struct {
	opts   *SlackOptions
	client *slack.Slack
	*base
}

// NewSlackLogger constructs a Sender that posts messages to a slack,
// given a slack API token. Configure the slack sender using a SlackOptions struct.
func NewSlackLogger(opts *SlackOptions, token string, l LevelInfo) (Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := &slackJournal{
		opts:   opts,
		client: slack.New(token),
		base:   newBase(opts.Name),
	}

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	if _, err := s.client.AuthTest(); err != nil {
		return nil, fmt.Errorf("slack authentication error: %v", err)
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s]", s.Name()))
	}

	s.SetName(opts.Name)

	return s, nil
}

// MakeSlackLogger is equivalent to NewSlackLogger, but constructs a
// Sender reading the slack token from the environment variable
// "GRIP_SLACK_CLIENT_TOKEN".
func MakeSlackLogger(opts *SlackOptions) (Sender, error) {
	token := os.Getenv(slackClientToken)
	if token == "" {
		return nil, fmt.Errorf("environment variable %s not defined, cannot create slack client",
			"foo")
	}

	return NewSlackLogger(opts, token, LevelInfo{level.Trace, level.Trace})
}

func (s *slackJournal) Send(m message.Composer) {
	if !s.level.ShouldLog(m) {
		return
	}

	msg := m.String()

	s.RLock()
	defer s.RUnlock()

	params := s.opts.getParams(m)
	if err := s.client.ChatPostMessage(s.opts.Channel, msg, params); err != nil {
		s.errHandler(err, message.NewFormattedMessage(m.Priority(),
			"%s: %s\n", params.Attachments[0].Fallback, msg))
	}
}

// SlackOptions configures the behavior for constructing messages sent
// to slack.
type SlackOptions struct {
	// Channel, Name, and Hostname are basic configurations
	// options for the sender, and control where the messages are
	// sent, which hostname the sender reports itself as, and the
	// name of the journaler.
	Channel  string
	Hostname string
	Name     string

	// Configuration options for appending structured data to the
	// message sent to slack. The BasicMetadata option appends
	// priority and host information to the message. The Fields
	// option appends structured data if the Raw method of a
	// Composer returns a message.Fields map. If you specify a set
	// of fields in the FieldsSet value, only those fields will be
	// attached to the message.
	BasicMetadata bool
	Fields        bool
	FieldsSet     map[string]struct{}
	mutex         sync.RWMutex
}

func (o *SlackOptions) fieldSetShouldInclude(name string) bool {
	if name == "time" {
		return false
	}

	o.mutex.RLock()
	defer o.mutex.RUnlock()

	if o.FieldsSet == nil {
		return true
	}

	_, ok := o.FieldsSet[name]

	return ok
}

// Validate inspects the contents SlackOptions struct and returns an
// error that reports on missing required fields (e.g. Channel and
// Name); if the hostname is missing and the os.Hostname() syscall
// fails (but supplies the Hostname as reported by Hostname if there is
// no Hostname is specified). Validate also prepends a missing "#" to
// the channel setting if the "#" character is not set.
func (o *SlackOptions) Validate() error {
	errs := []string{}
	if o.Channel == "" {
		errs = append(errs, "no channel specified")
	}

	if o.Name == "" {
		errs = append(errs, "no logger/journal name specified")
	}

	if o.FieldsSet == nil {
		o.FieldsSet = map[string]struct{}{}
	}

	if o.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			o.Hostname = hostname
		}
	}

	if !strings.HasPrefix(o.Channel, "#") {
		o.Channel = "#" + o.Channel
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (o *SlackOptions) getParams(m message.Composer) *slack.ChatPostMessageOpt {
	o.mutex.RLock()
	defer o.mutex.RUnlock()

	attachment := slack.Attachment{}
	fallbacks := []string{}

	p := m.Priority()
	if o.BasicMetadata {
		if o.Name != "" {
			fallbacks = append(fallbacks, fmt.Sprintf("journal=%s", o.Name))
			attachment.Fields = append(attachment.Fields,
				&slack.AttachmentField{
					Title: "Journal",
					Value: o.Name,
					Short: true,
				})

		}

		if o.Hostname != "!" && o.Hostname != "" {
			fallbacks = append(fallbacks, fmt.Sprintf("host=%s", o.Hostname))
			attachment.Fields = append(attachment.Fields,
				&slack.AttachmentField{
					Title: "Host",
					Value: o.Hostname,
					Short: true,
				})
		}

		fallbacks = append(fallbacks, fmt.Sprintf("priority=%s", p))
		attachment.Fields = append(attachment.Fields,
			&slack.AttachmentField{
				Title: "Priority",
				Value: p.String(),
				Short: true,
			})
	}

	if o.Fields {
		fields, ok := m.Raw().(message.Fields)

		if ok {
			for k, v := range fields {
				if k == "msg" && v == m.String() {
					continue
				}

				if o.fieldSetShouldInclude(k) {
					continue
				}

				fallbacks = append(fallbacks, fmt.Sprintf("%s=%v", k, v))

				attachment.Fields = append(attachment.Fields,
					&slack.AttachmentField{
						Title: k,
						Value: fmt.Sprintf("%v", v),
						Short: true,
					})
			}
		}
	}

	if len(fallbacks) > 0 {
		attachment.Fallback = fmt.Sprintf("[%s]", strings.Join(fallbacks, ", "))
	}

	switch p {
	case level.Emergency, level.Alert, level.Critical:
		attachment.Color = "danger"
	case level.Warning, level.Notice:
		attachment.Color = "warning"
	default: // includes info/debug
		attachment.Color = "good"

	}

	return &slack.ChatPostMessageOpt{
		Attachments: []*slack.Attachment{&attachment},
	}
}
