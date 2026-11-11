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

	// The maximum number of failed webhooks returned and retried per call, so a
	// single request cannot saturate queue workers.
	failedWebhookMaxLimit = 100
)

var (
	failedWebhookIDKey        = bsonutil.MustHaveTag(FailedWebhook{}, "NotificationID")
	failedWebhookProjectKey   = bsonutil.MustHaveTag(FailedWebhook{}, "Project")
	failedWebhookURLKey       = bsonutil.MustHaveTag(FailedWebhook{}, "URL")
	failedWebhookErrorKey     = bsonutil.MustHaveTag(FailedWebhook{}, "Error")
	failedWebhookFailedAtKey  = bsonutil.MustHaveTag(FailedWebhook{}, "FailedAt")
	failedWebhookRetriedAtKey = bsonutil.MustHaveTag(FailedWebhook{}, "RetriedAt")
	failedWebhookBodyKey      = bsonutil.MustHaveTag(FailedWebhook{}, "Body")
	failedWebhookHeadersKey   = bsonutil.MustHaveTag(FailedWebhook{}, "Headers")
)

type FailedWebhook struct {
	NotificationID string      `bson:"_id"`
	Project        string      `bson:"project,omitempty"`
	URL            string      `bson:"url"`
	Error          string      `bson:"error"`
	FailedAt       time.Time   `bson:"failed_at"`
	RetriedAt      time.Time   `bson:"retried_at,omitempty"`
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
	// Recover the delivery target from the subscriber when the composer never
	// ran, such as when webhook notifications are disabled.
	if sub, ok := n.Subscriber.Target.(*event.WebhookSubscriber); ok {
		if webhook.URL == "" {
			webhook.URL = sub.URL
		}
		// Only build the header set once so the notification's payload does not
		// accumulate duplicate headers when it fails repeatedly.
		if webhook.Headers == nil {
			webhook.Headers = http.Header{}
			for _, header := range sub.Headers {
				webhook.Headers.Add(header.Key, header.Value)
			}
		}
	}

	// The authorization header carries a credential, so the stored copy drops it.
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
	// A retried webhook that fails again collides with its existing record,
	// so upsert to refresh the failure and clear the retried mark.
	update := bson.M{
		"$set": bson.M{
			failedWebhookProjectKey:  failure.Project,
			failedWebhookURLKey:      failure.URL,
			failedWebhookErrorKey:    failure.Error,
			failedWebhookFailedAtKey: failure.FailedAt,
			failedWebhookBodyKey:     failure.Body,
			failedWebhookHeadersKey:  failure.Headers,
		},
		"$unset": bson.M{failedWebhookRetriedAtKey: ""},
	}
	if _, err := db.Upsert(ctx, FailedWebhookCollection, bson.M{failedWebhookIDKey: failure.NotificationID}, update); err != nil {
		return errors.Wrap(err, "upserting failed webhook")
	}

	return nil
}

// FindFailedWebhooks returns webhook deliveries that failed and have not been
// retried, up to failedWebhookMaxLimit.
func FindFailedWebhooks(ctx context.Context, projects []string) ([]FailedWebhook, error) {
	query := bson.M{
		failedWebhookProjectKey:   bson.M{"$in": projects},
		failedWebhookRetriedAtKey: bson.M{"$exists": false},
	}

	failedWebhooks := []FailedWebhook{}
	q := db.Query(query).Sort([]string{"-" + failedWebhookFailedAtKey}).Limit(failedWebhookMaxLimit)
	if err := db.FindAllQ(ctx, FailedWebhookCollection, q, &failedWebhooks); err != nil {
		return nil, errors.Wrap(err, "finding failed webhooks")
	}

	return failedWebhooks, nil
}
