package notification

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
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

	// failedWebhookRetryLease is how long a retry claim holds before the record
	// can be retried again.
	failedWebhookRetryLease = 5 * time.Minute
)

var (
	failedWebhookIDKey        = bsonutil.MustHaveTag(FailedWebhook{}, "NotificationID")
	failedWebhookProjectKey   = bsonutil.MustHaveTag(FailedWebhook{}, "Project")
	failedWebhookErrorKey     = bsonutil.MustHaveTag(FailedWebhook{}, "Error")
	failedWebhookFailedAtKey  = bsonutil.MustHaveTag(FailedWebhook{}, "FailedAt")
	failedWebhookRetriedAtKey = bsonutil.MustHaveTag(FailedWebhook{}, "RetriedAt")
)

type FailedWebhook struct {
	NotificationID string    `bson:"_id"`
	Project        string    `bson:"project,omitempty"`
	Error          string    `bson:"error"`
	FailedAt       time.Time `bson:"failed_at"`
	RetriedAt      time.Time `bson:"retried_at,omitempty"`
}

// InsertFailedWebhook records a webhook notification that failed to deliver.
func InsertFailedWebhook(ctx context.Context, n *Notification, sendErr error) error {
	if n == nil || sendErr == nil || n.Subscriber.Type != event.EvergreenWebhookSubscriberType || n.Project == "" {
		return nil
	}

	failure := FailedWebhook{
		NotificationID: n.ID,
		Project:        n.Project,
		Error:          sendErr.Error(),
		FailedAt:       time.Now().Truncate(time.Millisecond),
	}
	// A retried webhook that fails again collides with its existing record,
	// so upsert to refresh the failure and clear the retried mark.
	update := bson.M{
		"$set": bson.M{
			failedWebhookProjectKey:  failure.Project,
			failedWebhookErrorKey:    failure.Error,
			failedWebhookFailedAtKey: failure.FailedAt,
		},
		"$unset": bson.M{failedWebhookRetriedAtKey: ""},
	}
	if _, err := db.Upsert(ctx, FailedWebhookCollection, bson.M{failedWebhookIDKey: failure.NotificationID}, update); err != nil {
		return errors.Wrap(err, "upserting failed webhook")
	}

	return nil
}

// FindFailedWebhooks returns webhook deliveries that failed and are not
// claimed by an in-progress retry, up to failedWebhookMaxLimit.
func FindFailedWebhooks(ctx context.Context, projects []string) ([]FailedWebhook, error) {
	query := bson.M{
		failedWebhookProjectKey:   bson.M{"$in": projects},
		failedWebhookRetriedAtKey: unclaimedFailedWebhook(time.Now()),
	}

	failedWebhooks := []FailedWebhook{}
	q := db.Query(query).Sort([]string{"-" + failedWebhookFailedAtKey}).Limit(failedWebhookMaxLimit)
	if err := db.FindAllQ(ctx, FailedWebhookCollection, q, &failedWebhooks); err != nil {
		return nil, errors.Wrap(err, "finding failed webhooks")
	}

	return failedWebhooks, nil
}

// unclaimedFailedWebhook matches a retried_at that is unset or older than the
// retry lease.
func unclaimedFailedWebhook(now time.Time) bson.M {
	return bson.M{"$not": bson.M{"$gte": now.Add(-failedWebhookRetryLease)}}
}
