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
	NotificationsCollection = "notifications"
)

//nolint: deadcode, megacheck
var (
	idKey      = bsonutil.MustHaveTag(Notification{}, "ID")
	typeKey    = bsonutil.MustHaveTag(Notification{}, "Type")
	targetKey  = bsonutil.MustHaveTag(Notification{}, "Target")
	sentAtKey  = bsonutil.MustHaveTag(Notification{}, "SentAt")
	errorKey   = bsonutil.MustHaveTag(Notification{}, "Error")
	payloadKey = bsonutil.MustHaveTag(Notification{}, "Payload")
)

type Notification struct {
	ID      bson.ObjectId    `bson:"_id"`
	Type    string           `bson:"type"`
	Target  event.Subscriber `bson:"target"`
	Payload interface{}      `bson:"payload"`

	SentAt time.Time `bson:"sent_at,omitempty"`
	Error  string    `bson:"error,omitempty"`
}

type unmarshalNotification struct {
	ID      bson.ObjectId    `bson:"_id"`
	Type    string           `bson:"type"`
	Target  event.Subscriber `bson:"target"`
	Payload bson.Raw         `bson:"payload"`

	SentAt time.Time `bson:"sent_at,omitempty"`
	Error  string    `bson:"error,omitempty"`
}

func (n *Notification) SetBSON(raw bson.Raw) error {
	temp := unmarshalNotification{}
	if err := raw.Unmarshal(&temp); err != nil {
		return err
	}

	switch temp.Target.Type {
	case "evergreen-webhook":
		n.Payload = &EvergreenWebhookPayload{}

	case "email":
		n.Payload = &EmailPayload{}

	case "jira-issue":
		n.Payload = &JiraIssuePayload{}

	case "jira-comment":
		str := ""
		n.Payload = &str

	case "slack":
		n.Payload = &SlackPayload{}

	case "github_pull_request":
		n.Payload = &GithubStatusAPIPayload{}

	default:
		return errors.Errorf("unknown payload type %s", temp.Target.Type)
	}

	if err := temp.Payload.Unmarshal(n.Payload); err != nil {
		return errors.Errorf("error unmarshalling payload")
	}

	n.ID = temp.ID
	n.Type = temp.Type
	n.Target = temp.Target
	n.SentAt = temp.SentAt
	n.Error = temp.Error

	return nil
}

func (n *Notification) MarkSent() error {
	if !n.ID.Valid() {
		return errors.New("notification has no ID")
	}

	n.SentAt = time.Now().Truncate(time.Millisecond)

	update := bson.M{
		"$set": bson.M{
			sentAtKey: n.SentAt,
		},
	}

	if err := db.Update(NotificationsCollection, ByID(n.ID), update); err != nil {
		n.Error = ""
		return errors.New("failed to update notification")
	}

	return nil
}

func (n *Notification) MarkError(sendErr error) error {
	if sendErr == nil {
		return nil
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

	if err := db.Update(NotificationsCollection, ByID(n.ID), update); err != nil {
		n.Error = ""
		return errors.New("failed to add error to notification")
	}

	return nil
}

func (n *Notification) Composer() (message.Composer, error) {
	// TODO relocate and use constants
	switch n.Target.Type {
	case "evergreen-webhook":
		return nil, fmt.Errorf("evergreen-webhook does not use a composer")

	case "email":
		// grip email sender doesn't actually use the composer, one is
		// required by Send(), so:
		c := message.NewString("")
		c.SetPriority(level.Notice)

		return c, nil

	case "jira-issue":
		project, ok := n.Target.Target.(string)
		if !ok {
			return nil, errors.New("jira-issue subscriber is invalid")
		}
		payload, ok := n.Payload.(JiraIssuePayload)
		if !ok {
			return nil, errors.New("jira-issue payload is invalid")
		}

		return message.MakeJiraMessage(message.JiraIssue{
			Project:     project,
			Summary:     payload.Summary,
			Description: payload.Description,
			Reporter:    payload.Reporter,
			Assignee:    payload.Assignee,
			Type:        payload.Type,
			Components:  payload.Components,
			Labels:      payload.Labels,
			Fields:      payload.Fields,
		}), nil

	case "jira-comment":
		payload, ok := n.Payload.(string)
		if !ok {
			return nil, errors.New("jira-comment payload is invalid")
		}

		c := message.NewString(payload)
		c.SetPriority(level.Notice)

		return c, nil

	case "slack":
		// TODO figure out slack message structure
		return message.ConvertToComposer(level.Notice, message.Fields{}), nil

	case "github_pull_request":
		payload, ok := n.Payload.(GithubStatusAPIPayload)
		if !ok {
			return nil, errors.New("github-pull-request payload is invalid")
		}

		return message.ConvertToComposer(level.Notice, message.Fields{
			"url":         payload.URL,
			"context":     payload.Context,
			"status":      payload.Status,
			"description": payload.Description,
		}), nil

	default:
		return nil, errors.Errorf("unknown type '%s'", n.Target.Type)
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

	return db.InsertMany(NotificationsCollection, interfaces...)
}

func ByID(id bson.ObjectId) db.Q {
	return db.Query(bson.M{
		idKey: id,
	})
}

func Find(id bson.ObjectId) (*Notification, error) {
	notification := Notification{}
	err := db.FindOneQ(NotificationsCollection, ByID(id), &notification)

	if err == mgo.ErrNotFound {
		return nil, nil
	}

	return &notification, err
}
