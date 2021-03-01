package notification

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	Collection = "notifications"
)

//nolint: deadcode, megacheck, unused
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
	Payload    mgobson.Raw      `bson:"payload"`

	SentAt   time.Time            `bson:"sent_at,omitempty"`
	Error    string               `bson:"error,omitempty"`
	Metadata NotificationMetadata `bson:"metadata,omitempty"`
}

func (d *Notification) UnmarshalBSON(in []byte) error {
	return mgobson.Unmarshal(in, d)
}

func (n *Notification) SetBSON(raw mgobson.Raw) error {
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

	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType:
		n.Payload = &message.GithubStatus{}
	case event.GithubMergeSubscriberType:
		n.Payload = &commitqueue.GithubMergePR{}

	case event.EnqueuePatchSubscriberType:
		n.Payload = &model.EnqueuePatch{}

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
	n.Metadata = temp.Metadata

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

	// notification IDs are intended to collide when multiple subscriptions exist to the same event
	// insert unordered will continue on error so the rest of the notifications in items will still be inserted
	return db.InsertManyUnordered(Collection, interfaces...)
}

func Find(id string) (*Notification, error) {
	notification := Notification{}
	err := db.FindOneQ(Collection, byID(id), &notification)

	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &notification, err
}

func FindByEventID(id string) ([]Notification, error) {
	notifications := []Notification{}
	query := db.Query(bson.M{
		idKey: primitive.Regex{Pattern: fmt.Sprintf("^%s-", id)},
	},
	)

	err := db.FindAllQ(Collection, query, &notifications)
	return notifications, err
}

func FindUnprocessed() ([]Notification, error) {
	notifications := []Notification{}
	err := db.FindAllQ(Collection, db.Query(bson.M{sentAtKey: bson.M{"$exists": false}}), &notifications)

	return notifications, errors.Wrap(err, "can't query for unprocessed notifications")
}

func byID(id string) db.Q {
	return db.Query(bson.M{
		idKey: id,
	})
}
