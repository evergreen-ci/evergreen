package notification

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip/level"
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

// makeNotificationID creates a string representing the notification generated
// from the given event, with the given trigger, for the given subscriber.
// This function will produce an ID that will collide to prevent duplicate
// notifications from being inserted
func makeNotificationID(event *event.EventLogEntry, trigger string, subscriber *event.Subscriber) string {
	return fmt.Sprintf("%s-%s-%s", event.ID.Hex(), trigger, subscriber.String())
}

// New returns a new Notification, with a correctly initialised ID
func New(e *event.EventLogEntry, trigger string, subscriber *event.Subscriber, payload interface{}) (*Notification, error) {
	if e == nil {
		return nil, errors.New("cannot create notification from nil event")
	}
	if len(trigger) == 0 {
		return nil, errors.New("cannot create notification from nil trigger")
	}
	if subscriber == nil {
		return nil, errors.New("cannot create notification from nil subscriber")
	}
	if payload == nil {
		return nil, errors.New("cannot create notification with nil payload")
	}

	return &Notification{
		ID:         makeNotificationID(e, trigger, subscriber),
		Subscriber: *subscriber,
		Payload:    payload,
	}, nil
}

type Notification struct {
	ID         string           `bson:"_id"`
	Subscriber event.Subscriber `bson:"subscriber"`
	Payload    interface{}      `bson:"payload"`

	SentAt time.Time `bson:"sent_at,omitempty"`
	Error  string    `bson:"error,omitempty"`
}

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
		str := ""
		n.Payload = &str

	case event.EmailSubscriberType:
		n.Payload = &EmailPayload{}

	case event.JIRAIssueSubscriberType:
		n.Payload = &message.JiraIssue{}

	case event.JIRACommentSubscriberType:
		str := ""
		n.Payload = &str

	case event.SlackSubscriberType:
		str := ""
		n.Payload = &str

	case event.GithubPullRequestSubscriberType:
		n.Payload = &GithubStatusAPIPayload{}

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

func (n *Notification) MarkSent() error {
	if len(n.ID) == 0 {
		return errors.New("notification has no ID")
	}

	n.SentAt = time.Now().Truncate(time.Millisecond)

	update := bson.M{
		"$set": bson.M{
			sentAtKey: n.SentAt,
		},
	}

	if err := db.UpdateId(Collection, n.ID, update); err != nil {
		return errors.Wrap(err, "failed to update notification")
	}

	return nil
}

func (n *Notification) MarkError(sendErr error) error {
	if sendErr == nil {
		return nil
	}
	if len(n.ID) == 0 {
		return errors.New("notification has no ID")
	}
	if n.SentAt.IsZero() {
		if err := n.MarkSent(); err != nil {
			return err
		}
	}

	errMsg := sendErr.Error()
	update := bson.M{
		"$set": bson.M{
			errorKey: errMsg,
		},
	}
	n.Error = errMsg

	if err := db.UpdateId(Collection, n.ID, update); err != nil {
		n.Error = ""
		return errors.Wrap(err, "failed to add error to notification")
	}

	return nil
}

func (n *Notification) Composer() (message.Composer, error) {
	switch n.Subscriber.Type {
	case event.EvergreenWebhookSubscriberType:
		payload, ok := n.Payload.(*string)
		if !ok || payload == nil {
			return nil, errors.New("evergreen-webhook payload is invalid")
		}
		c := message.NewString(*payload)
		if err := c.SetPriority(level.Notice); err != nil {
			return nil, errors.Wrap(err, "failed to set priority")
		}

		return c, nil

	case event.EmailSubscriberType:
		// TODO make real composer for this
		payload, ok := n.Payload.(*EmailPayload)
		if !ok || payload == nil {
			return nil, errors.New("email payload is invalid")
		}

		return message.ConvertToComposer(level.Notice, message.Fields{
			"headers": payload.Headers,
			"subject": payload.Subject,
			"body":    payload.Body,
		}), nil

	case event.JIRAIssueSubscriberType:
		project, ok := n.Subscriber.Target.(*string)
		if !ok {
			return nil, errors.New("jira-issue subscriber is invalid")
		}
		payload, ok := n.Payload.(*message.JiraIssue)
		if !ok || payload == nil {
			return nil, errors.New("jira-issue payload is invalid")
		}

		payload.Project = *project

		return message.MakeJiraMessage(*payload), nil

	case event.JIRACommentSubscriberType:
		payload, ok := n.Payload.(*string)
		if !ok || payload == nil {
			return nil, errors.New("jira-comment payload is invalid")
		}

		c := message.NewString(*payload)
		if err := c.SetPriority(level.Notice); err != nil {
			return nil, errors.Wrap(err, "failed to set priority")
		}

		return c, nil

	case event.SlackSubscriberType:
		// TODO figure out slack message structure that works
		payload, ok := n.Payload.(*string)
		if !ok || payload == nil {
			return nil, errors.New("slack payload is invalid")
		}

		c := message.NewString(*payload)
		if err := c.SetPriority(level.Notice); err != nil {
			return nil, errors.Wrap(err, "failed to set priority")
		}

		return c, nil

	case event.GithubPullRequestSubscriberType:
		// TODO make real composer for this
		payload, ok := n.Payload.(*GithubStatusAPIPayload)
		if !ok || payload == nil {
			return nil, errors.New("github-pull-request payload is invalid")
		}

		return message.ConvertToComposer(level.Notice, message.Fields{
			"url":         payload.URL,
			"context":     payload.Context,
			"status":      payload.Status,
			"description": payload.Description,
		}), nil

	default:
		return nil, errors.Errorf("unknown type '%s'", n.Subscriber.Type)
	}
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
