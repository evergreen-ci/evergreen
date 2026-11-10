package notification

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// FailedWebhookCollection stores webhook notifications that failed to deliver.
	FailedWebhookCollection = "failed_webhooks"

	// The maximum number of failed webhooks returned when the caller does not provide a positive limit.
	failedWebhookMaxLimit = 500
)

var (
	failedWebhookProjectKey  = bsonutil.MustHaveTag(FailedWebhook{}, "Project")
	failedWebhookURLKey      = bsonutil.MustHaveTag(FailedWebhook{}, "URL")
	failedWebhookFailedAtKey = bsonutil.MustHaveTag(FailedWebhook{}, "FailedAt")
)

type FailedWebhook struct {
	NotificationID string      `bson:"_id"`
	Project        string      `bson:"project,omitempty"`
	URL            string      `bson:"url"`
	Error          string      `bson:"error"`
	FailedAt       time.Time   `bson:"failed_at"`
	Body           []byte      `bson:"body,omitempty"`
	Headers        http.Header `bson:"headers,omitempty"`
}

// InsertFailedWebhook records a webhook notification that failed to deliver.
func InsertFailedWebhook(ctx context.Context, n *Notification, sendErr error) error {
	if n == nil || sendErr == nil || n.Subscriber.Type != event.EvergreenWebhookSubscriberType || n.Project == "" {
		return nil
	}
	webhook, ok := n.Payload.(*util.EvergreenWebhook)
	if !ok || webhook == nil {
		return nil
	}
	// Handle degraded mode.
	if webhook.URL == "" || webhook.Headers == nil {
		if sub, ok := n.Subscriber.Target.(*event.WebhookSubscriber); ok {
			if webhook.URL == "" {
				webhook.URL = sub.URL
			}
			if webhook.Headers == nil {
				webhook.Headers = http.Header{}
			}
			for _, header := range sub.Headers {
				webhook.Headers.Add(header.Key, header.Value)
			}
		}
	}

	// Delete auth details.
	headers := webhook.Headers.Clone()
	headers.Del(event.WebhookAuthorizationHeader)

	failure := FailedWebhook{
		NotificationID: n.ID,
		Project:        n.Project,
		URL:            webhook.URL,
		Error:          sendErr.Error(),
		FailedAt:       time.Now().Truncate(time.Millisecond),
		Body:           webhook.Body,
		Headers:        headers,
	}
	if err := db.Insert(ctx, FailedWebhookCollection, &failure); err != nil {
		if db.IsDuplicateKey(err) {
			return nil
		}
		return errors.Wrap(err, "inserting failed webhook")
	}

	return nil
}

// FindFailedWebhooks returns failed webhook deliveries for the given projects.
// The limit is capped at failedWebhookMaxLimit.
func FindFailedWebhooks(ctx context.Context, projects []string, url string, limit int) ([]FailedWebhook, error) {
	query := bson.M{
		failedWebhookProjectKey: bson.M{"$in": projects},
	}
	if url != "" {
		query[failedWebhookURLKey] = url
	}

	if limit <= 0 || limit > failedWebhookMaxLimit {
		limit = failedWebhookMaxLimit
	}

	failedWebhooks := []FailedWebhook{}
	q := db.Query(query).Sort([]string{"-" + failedWebhookFailedAtKey}).Limit(limit)
	if err := db.FindAllQ(ctx, FailedWebhookCollection, q, &failedWebhooks); err != nil {
		return nil, errors.Wrap(err, "finding failed webhooks")
	}

	return failedWebhooks, nil
}
