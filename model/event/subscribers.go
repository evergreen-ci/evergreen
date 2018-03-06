package event

import (
	"net/url"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	githubPullRequestSubscriberType = "github_pull_request"
	webhookSubscriberType           = "webhook"
)

var (
	subscriberTypeKey   = bsonutil.MustHaveTag(Subscriber{}, "Type")
	subscriberTargetKey = bsonutil.MustHaveTag(Subscriber{}, "Target")
)

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
	if len(temp.Type) == 0 {
		return errors.New("could not find subscriber type")
	}
	s.Type = temp.Type

	switch s.Type {
	case githubPullRequestSubscriberType:
		s.Target = githubPullRequestSubscriber{}
	case webhookSubscriberType:
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
