package event

import (
	"net/url"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	ResourceTypeNotification = "notification"
)

var (
	subscriberTypeKey   = bsonutil.MustHaveTag(Subscriber{}, "Type")
	subscriberTargetKey = bsonutil.MustHaveTag(Subscriber{}, "Target")
)

type NotificationEvent struct {
	ResourceType string           `bson:"r_type" json:"resource_type"`
	Type         string           `bson:"type" json:"type"`
	Undelivered  []Subscriber     `bson:"undelivered" json:"undelivered"`
	Delivered    []Subscriber     `bson:"delivered" json:"delivered"`
	Payload      message.Composer `bson:"payload" json:"payload"`
}

func (e *NotificationEvent) IsValid() bool {
	return e.ResourceType == ResourceTypeNotification
}

type Subscriber struct {
	Type string `bson:"type"`
	// sad violin
	Target interface{} `bson:"target"`
}

func (s *Subscriber) SetBSON(raw bson.Raw) error {
	temp := struct {
		Type   string `bson:"type"`
		Target []byte `bson:"target"`
	}{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal subscriber data")
	}
	s.Type = temp.Type

	if len(s.Type) == 0 {
		return errors.New("could not find subscriber type")
	}

	// TODO registry?
	switch s.Type {
	case "github_pull_request":
		s.Target = githubPullRequestSubscriber{}
	case "webhook":
		s.Target = webhookSubscriber{}
	default:
		s.Target = string(temp.Target)
		return nil
	}

	if err := bson.Unmarshal(temp.Target, s.Target); err != nil {
		return errors.Wrap(err, "couldn't unmarshal subscriber info")
	}

	return nil
}

type stringSubscriber struct {
	Data string `bson:"data"`
}

type webhookSubscriber struct {
	URL    url.URL `bson:"url"`
	Secret []byte  `bson:"secret"`
}

type githubPullRequestSubscriber struct {
	Owner    string `bson:"owner"`
	Repo     string `bson:"repo"`
	PRNumber int    `bson:"pr_number"`
	Ref      string `bson:"ref"`
}
