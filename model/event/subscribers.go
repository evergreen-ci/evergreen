package event

import (
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

type WebhookSubscriber struct {
	URL    string `bson:"url"`
	Secret []byte `bson:"secret"`
}

type GithubPullRequestSubscriber struct {
	Owner    string `bson:"owner"`
	Repo     string `bson:"repo"`
	PRNumber int    `bson:"pr_number"`
	Ref      string `bson:"ref"`
}
