package notification

import (
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgo "gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	Collection = "notifications"
)

//nolint: deadcode, megacheck
var (
	idKey         = bsonutil.MustHaveTag(Notification{}, "ID")
	subscriberKey = bsonutil.MustHaveTag(Notification{}, "Subscriber")
	payloadKey    = bsonutil.MustHaveTag(Notification{}, "Payload")
	sentAtKey     = bsonutil.MustHaveTag(Notification{}, "SentAt")
	errorKey      = bsonutil.MustHaveTag(Notification{}, "Error")
)

type unmarshalNotification struct {
	ID         string           `bson:"_id"`
	Subscriber event.Subscriber `bson:"subscriber"`
	Payload    bson.Raw         `bson:"payload"`

	SentAt time.Time `bson:"sent_at,omitempty"`
	Error  string    `bson:"error,omitempty"`
}

func (n *Notification) SetBSON(raw bson.Raw) error {
	temp := unmarshalNotification{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "can't unmarshal notification")
	}

	switch temp.Subscriber.Type {
	case event.EvergreenWebhookSubscriberType:
		n.Payload = &util.EvergreenWebhook{}

	case event.EmailSubscriberType:
		n.Payload = &message.Email{}

	case event.JIRAIssueSubscriberType:
		n.Payload = &message.JiraIssue{}

	case event.JIRACommentSubscriberType:
		str := ""
		n.Payload = &str

	case event.SlackSubscriberType:
		n.Payload = &SlackPayload{}

	case event.GithubPullRequestSubscriberType:
		n.Payload = &message.GithubStatus{}

	default:
		return errors.Errorf("unknown payload type %s", temp.Subscriber.Type)
	}

	if err := temp.Payload.Unmarshal(n.Payload); err != nil {
		return errors.Wrap(err, "error unmarshalling payload")
	}

	n.ID = temp.ID
	n.Subscriber = temp.Subscriber
	n.SentAt = temp.SentAt
	n.Error = temp.Error

	return nil
}

func InsertMany(items ...Notification) error {
	if len(items) == 0 {
		return nil
	}

	interfaces := make([]interface{}, len(items))
	for i := range items {
		interfaces[i] = &items[i]
	}

	return db.InsertMany(Collection, interfaces...)
}

func ByID(id string) db.Q {
	return db.Query(bson.M{
		idKey: id,
	})
}

func Find(id string) (*Notification, error) {
	notification := Notification{}
	err := db.FindOneQ(Collection, ByID(id), &notification)

	if err == mgo.ErrNotFound {
		return nil, nil
	}

	return &notification, err
}
