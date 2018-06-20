package send

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"

	"github.com/bluele/slack"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

const (
	slackClientToken = "GRIP_SLACK_CLIENT_TOKEN"
)

type slackJournal struct {
	opts *SlackOptions
	*Base
}

// NewSlackLogger constructs a Sender that posts messages to a slack,
// given a slack API token. Configure the slack sender using a SlackOptions struct.
func NewSlackLogger(opts *SlackOptions, token string, l LevelInfo) (Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := &slackJournal{
		opts: opts,
		Base: NewBase(opts.Name),
	}

	s.opts.client.Create(token)

	if err := s.SetLevel(l); err != nil {
		return nil, err
	}

	if _, err := s.opts.client.AuthTest(); err != nil {
		return nil, fmt.Errorf("slack authentication error: %v", err)
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
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
			slackClientToken)
	}

	return NewSlackLogger(opts, token, LevelInfo{level.Trace, level.Trace})
}

func (s *slackJournal) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		s.Base.mutex.RLock()
		defer s.Base.mutex.RUnlock()

		var msg string
		var params *slack.ChatPostMessageOpt
		channel := s.opts.Channel

		if slackMsg, ok := m.Raw().(*message.Slack); ok {
			channel = slackMsg.Target
			msg, params = slackMsg.Msg, &slack.ChatPostMessageOpt{
				Attachments: slackMsg.Attachments,
			}

		} else {
			msg, params = s.opts.produceMessage(m)
		}

		params.IconUrl = s.opts.IconURL
		params.Username = s.opts.Username
		if len(params.Username) != 0 || len(params.IconUrl) != 0 {
			params.AsUser = false
		}

		if err := s.opts.client.ChatPostMessage(channel, msg, params); err != nil {
			s.ErrorHandler(err, message.NewFormattedMessage(m.Priority(),
				"%s\n", msg))
		}
	}
}

// SlackOptions configures the behavior for constructing messages sent
// to slack.
type SlackOptions struct {
	// Channel, Name, and Hostname are basic configurations
	// options for the sender, and control where the messages are
	// sent, which hostname the sender reports itself as, and the
	// name of the journaler.
	Channel  string `bson:"channel" json:"channel" yaml:"channel"`
	Hostname string `bson:"hostname" json:"hostname" yaml:"hostname"`
	Name     string `bson:"name" json:"name" yaml:"name"`
	// Username and IconURL allow the slack sender to set a display
	// name and icon. Setting either parameter will force as_user to false.
	Username string `bson:"username" json:"username" yaml:"username"`
	IconURL  string `bson:"icon_url" json:"icon_url" yaml:"icon_url"`

	// Configuration options for appending structured data to the
	// message sent to slack. The BasicMetadata option appends
	// priority and host information to the message. The Fields
	// option appends structured data if the Raw method of a
	// Composer returns a message.Fields map. If you specify a set
	// of fields in the FieldsSet value, only those fields will be
	// attached to the message.
	BasicMetadata bool            `bson:"add_basic_metadata" json:"add_basic_metadata" yaml:"add_basic_metadata"`
	Fields        bool            `bson:"use_fields" json:"use_fields" yaml:"use_fields"`
	AllFields     bool            `bson:"all_fields" json:"all_fields" yaml:"all_fields"`
	FieldsSet     map[string]bool `bson:"fields" json:"fields" yaml:"fields"`

	client slackClient
	mutex  sync.RWMutex
}

func (o *SlackOptions) fieldSetShouldInclude(name string) bool {
	if name == "time" || name == "metadata" {
		return false
	}

	o.mutex.RLock()
	defer o.mutex.RUnlock()

	if o.AllFields || o.FieldsSet == nil {
		return true
	}

	return o.FieldsSet[name]
}

// Validate inspects the contents SlackOptions struct and returns an
// error that reports on missing required fields (e.g. Channel and
// Name); if the hostname is missing and the os.Hostname() syscall
// fails (but supplies the Hostname as reported by Hostname if there is
// no Hostname is specified). Validate also prepends a missing "#" to
// the channel setting if the "#" character is not set.
func (o *SlackOptions) Validate() error {
	if o == nil {
		return errors.New("slack options cannot be nil")
	}

	errs := []string{}
	if o.Channel == "" {
		errs = append(errs, "no channel specified")
	}

	if o.Name == "" {
		errs = append(errs, "no logger/journal name specified")
	}

	if o.FieldsSet == nil {
		o.FieldsSet = map[string]bool{}
	}
	if o.client == nil {
		o.client = &slackClientImpl{}
	}

	if o.Hostname == "" {
		hostname, err := os.Hostname()
		if err != nil {
			errs = append(errs, err.Error())
		} else {
			o.Hostname = hostname
		}
	}

	if !strings.HasPrefix(o.Channel, "#") && !strings.HasPrefix(o.Channel, "@") {
		return errors.New("Recipient must begin with '#' or '@'")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func (o *SlackOptions) produceMessage(m message.Composer) (string, *slack.ChatPostMessageOpt) {
	var msg string

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
				Title: "priority",
				Value: p.String(),
				Short: true,
			})
	}

	if o.Fields {
		fields, ok := m.Raw().(message.Fields)
		if ok {
			for k, v := range fields {
				if !o.fieldSetShouldInclude(k) {
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
		} else {
			msg = m.String()
		}
	} else {
		msg = m.String()
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

	return msg, &slack.ChatPostMessageOpt{
		Attachments: []*slack.Attachment{&attachment},
	}
}

////////////////////////////////////////////////////////////////////////
//
// interface wrapper for the slack client so that we can mock things out
//
////////////////////////////////////////////////////////////////////////

type slackClient interface {
	Create(string)
	AuthTest() (*slack.AuthTestApiResponse, error)
	ChatPostMessage(string, string, *slack.ChatPostMessageOpt) error
}

type slackClientImpl struct {
	*slack.Slack
}

func (c *slackClientImpl) Create(token string) {
	c.Slack = slack.New(token)
}
