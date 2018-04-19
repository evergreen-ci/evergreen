package event

import (
	"fmt"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	GithubPullRequestSubscriberType = "github_pull_request"
	JIRAIssueSubscriberType         = "jira-issue"
	JIRACommentSubscriberType       = "jira-comment"
	EvergreenWebhookSubscriberType  = "evergreen-webhook"
	EmailSubscriberType             = "email"
	SlackSubscriberType             = "slack"
)

//nolint: deadcode, megacheck
var (
	subscriberTypeKey   = bsonutil.MustHaveTag(Subscriber{}, "Type")
	subscriberTargetKey = bsonutil.MustHaveTag(Subscriber{}, "Target")
)

type Subscriber struct {
	Type string `bson:"type"`
	// sad violin
	Target interface{} `bson:"target"`
}

type unmarshalSubscriber struct {
	Type   string   `bson:"type"`
	Target bson.Raw `bson:"target"`
}

func (s *Subscriber) SetBSON(raw bson.Raw) error {
	temp := unmarshalSubscriber{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal subscriber data")
	}
	if len(temp.Type) == 0 {
		return errors.New("could not find subscriber type")
	}
	s.Type = temp.Type

	switch s.Type {
	case GithubPullRequestSubscriberType:
		s.Target = &GithubPullRequestSubscriber{}

	case EvergreenWebhookSubscriberType:
		s.Target = &WebhookSubscriber{}

	case JIRAIssueSubscriberType, JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType:
		str := ""
		s.Target = &str

	default:
		return errors.Errorf("unknown subscriber type: '%s'", s.Type)
	}

	if err := temp.Target.Unmarshal(s.Target); err != nil {
		return errors.Wrap(err, "couldn't unmarshal subscriber info")
	}

	return nil
}

func (s *Subscriber) String() string {
	subscriberStr := "NIL_SUBSCRIBER"

	switch v := s.Target.(type) {
	case GithubPullRequestSubscriber:
		subscriberStr = v.String()
	case *GithubPullRequestSubscriber:
		subscriberStr = v.String()

	case WebhookSubscriber:
		subscriberStr = v.String()
	case *WebhookSubscriber:
		subscriberStr = v.String()

	case string:
		subscriberStr = v
	case *string:
		subscriberStr = *v
	}

	return fmt.Sprintf("%s-%s", s.Type, subscriberStr)
}

type WebhookSubscriber struct {
	URL    string `bson:"url"`
	Secret []byte `bson:"secret"`
}

func (s *WebhookSubscriber) String() string {
	if len(s.URL) == 0 {
		return "NIL_URL"
	}
	return s.URL
}

type GithubPullRequestSubscriber struct {
	Owner    string `bson:"owner"`
	Repo     string `bson:"repo"`
	PRNumber int    `bson:"pr_number"`
	Ref      string `bson:"ref"`
}

func (s *GithubPullRequestSubscriber) String() string {
	return fmt.Sprintf("%s-%s-%d-%s", s.Owner, s.Repo, s.PRNumber, s.Ref)
}
