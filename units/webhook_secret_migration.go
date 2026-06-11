package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	webhookSecretMigrationJobName   = "webhook-secret-migration"
	webhookSecretMigrationBatchSize = 25

	webhookSecretCleanupJobName   = "webhook-secret-cleanup"
	webhookSecretCleanupBatchSize = 25
)

func init() {
	registry.AddJobType(webhookSecretMigrationJobName,
		func() amboy.Job { return makeWebhookSecretMigrationJob() })
	registry.AddJobType(webhookSecretCleanupJobName,
		func() amboy.Job { return makeWebhookSecretCleanupJob() })
}

type webhookSecretMigrationJob struct {
	job.Base       `bson:"job_base" json:"job_base" yaml:"job_base"`
	SubscriptionID string `bson:"subscription_id" json:"subscription_id" yaml:"subscription_id"`

	env evergreen.Environment
}

func makeWebhookSecretMigrationJob() *webhookSecretMigrationJob {
	j := &webhookSecretMigrationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    webhookSecretMigrationJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewWebhookSecretMigrationJob creates a job to migrate a single webhook
// subscription's secret from MongoDB to Parameter Store.
func NewWebhookSecretMigrationJob(subscriptionID, ts string) amboy.Job {
	j := makeWebhookSecretMigrationJob()
	j.SubscriptionID = subscriptionID
	j.SetID(fmt.Sprintf("%s.%s.%s", webhookSecretMigrationJobName, subscriptionID, ts))
	j.SetScopes([]string{
		fmt.Sprintf("%s.%s", webhookSecretMigrationJobName, subscriptionID),
	})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *webhookSecretMigrationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}
	if !flags.WebhookSecretMigrationEnabled {
		return
	}

	// Query as a guard: ResultsNotFound means there is nothing to migrate (not
	// found, wrong type, or both secret and Authorization header already migrated).
	sub := &event.Subscription{}
	if err := db.FindOneQ(ctx, event.SubscriptionsCollection, db.Query(append(bson.D{{Key: "_id", Value: j.SubscriptionID}}, unmigratedWebhookQuery...)), sub); err != nil {
		if !adb.ResultsNotFound(err) {
			j.AddError(errors.Wrapf(err, "finding subscription '%s'", j.SubscriptionID))
		}
		return
	}

	webhookSub, ok := sub.Subscriber.Target.(*event.WebhookSubscriber)
	if !ok {
		return
	}

	paramMgr := j.env.ParameterManager()

	if len(webhookSub.Secret) > 0 && webhookSub.SecretParameter == "" {
		paramPath := event.GetWebhookSecretParameterPath(j.SubscriptionID)
		param, err := paramMgr.Put(ctx, paramPath, string(webhookSub.Secret))
		if err != nil {
			j.AddError(errors.Wrapf(err, "saving webhook secret to Parameter Store for subscription '%s'", j.SubscriptionID))
			return
		}
		// Verify the value in Parameter Store matches before updating MongoDB, so
		// we never lose the only copy of the secret if the write was silently corrupted.
		validated, err := paramMgr.GetStrict(ctx, param.Name)
		if err != nil {
			j.AddError(errors.Wrapf(err, "verifying webhook secret in Parameter Store for subscription '%s'", j.SubscriptionID))
			return
		}
		if len(validated) == 0 || validated[0].Value != string(webhookSub.Secret) {
			j.AddError(errors.Errorf("webhook secret mismatch for subscription '%s': Parameter Store value does not match original", j.SubscriptionID))
			return
		}
		if err := db.Update(ctx, event.SubscriptionsCollection,
			bson.D{{Key: "_id", Value: j.SubscriptionID}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "subscriber.target.secret_parameter", Value: param.Name}}}},
		); err != nil {
			j.AddError(errors.Wrapf(err, "updating subscription '%s' after migrating secret", j.SubscriptionID))
			return
		}
		grip.Info(ctx, message.Fields{
			"message":         "successfully migrated webhook secret to Parameter Store",
			"subscription_id": j.SubscriptionID,
			"job_id":          j.ID(),
		})
	}

	if authValue := webhookSub.GetHeader("Authorization"); authValue != "" && webhookSub.AuthorizationHeaderParameter == "" {
		paramPath := event.GetWebhookAuthParameterPath(j.SubscriptionID)
		param, err := paramMgr.Put(ctx, paramPath, authValue)
		if err != nil {
			j.AddError(errors.Wrapf(err, "saving webhook Authorization header to Parameter Store for subscription '%s'", j.SubscriptionID))
			return
		}
		validated, err := paramMgr.GetStrict(ctx, param.Name)
		if err != nil {
			j.AddError(errors.Wrapf(err, "verifying webhook Authorization header in Parameter Store for subscription '%s'", j.SubscriptionID))
			return
		}
		if len(validated) == 0 || validated[0].Value != authValue {
			j.AddError(errors.Errorf("webhook Authorization header mismatch for subscription '%s': Parameter Store value does not match original", j.SubscriptionID))
			return
		}
		if err := db.Update(ctx, event.SubscriptionsCollection,
			bson.D{{Key: "_id", Value: j.SubscriptionID}},
			bson.D{{Key: "$set", Value: bson.D{{Key: "subscriber.target.authorization_header_parameter", Value: param.Name}}}},
		); err != nil {
			j.AddError(errors.Wrapf(err, "updating subscription '%s' after migrating Authorization header", j.SubscriptionID))
			return
		}
		grip.Info(ctx, message.Fields{
			"message":         "successfully migrated webhook Authorization header to Parameter Store",
			"subscription_id": j.SubscriptionID,
			"job_id":          j.ID(),
		})
	}
}

// unmigratedWebhookQuery matches webhook subscriptions that still have their
// secret or Authorization header stored in MongoDB and need migration to Parameter Store.
var unmigratedWebhookQuery = bson.D{
	{Key: "subscriber.type", Value: event.EvergreenWebhookSubscriberType},
	{Key: "$or", Value: bson.A{
		bson.D{
			{Key: "subscriber.target.secret", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$ne", Value: nil}}},
			{Key: "subscriber.target.secret_parameter", Value: bson.D{{Key: "$exists", Value: false}}},
		},
		bson.D{
			{Key: "subscriber.target.headers", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "key", Value: "Authorization"},
				{Key: "value", Value: bson.D{{Key: "$ne", Value: ""}}},
			}}}},
			{Key: "subscriber.target.authorization_header_parameter", Value: bson.D{{Key: "$exists", Value: false}}},
		},
	}},
}

// findUnmigratedWebhookSubscriptionIDs returns IDs of webhook subscriptions
// that still have their secret or Authorization header stored in MongoDB (not
// yet migrated to Parameter Store).
func findUnmigratedWebhookSubscriptionIDs(ctx context.Context, limit int) ([]string, error) {
	subscriptions := []event.Subscription{}
	if err := db.FindAllQ(ctx, event.SubscriptionsCollection, db.Query(unmigratedWebhookQuery).Limit(limit), &subscriptions); err != nil {
		return nil, errors.Wrap(err, "finding unmigrated webhook subscriptions")
	}

	ids := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		ids = append(ids, sub.ID)
	}
	return ids, nil
}

// countUnmigratedWebhookSubscriptions returns the total number of webhook
// subscriptions that have not yet been migrated to Parameter Store.
func countUnmigratedWebhookSubscriptions(ctx context.Context) (int, error) {
	return db.CountQ(ctx, event.SubscriptionsCollection, db.Query(unmigratedWebhookQuery))
}

// PopulateWebhookSecretMigrationJobs returns a QueueOperation that enqueues
// jobs to migrate webhook secrets from MongoDB to Parameter Store.
func PopulateWebhookSecretMigrationJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if !flags.WebhookSecretMigrationEnabled {
			return nil
		}

		ids, err := findUnmigratedWebhookSubscriptionIDs(ctx, webhookSecretMigrationBatchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			grip.Info(ctx, message.Fields{
				"message": "webhook secret migration complete: no unmigrated subscriptions remaining",
			})
			return nil
		}

		remainingCount, err := countUnmigratedWebhookSubscriptions(ctx)
		if err != nil {
			grip.Warning(ctx, message.WrapError(err, message.Fields{
				"message": "counting remaining unmigrated subscriptions",
			}))
		}

		grip.Info(ctx, message.Fields{
			"message":                    "enqueuing webhook secret migration jobs",
			"batch_count":                len(ids),
			"remaining_unmigrated_count": remainingCount,
		})

		ts := utility.RoundPartOfHour(5).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		for _, id := range ids {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewWebhookSecretMigrationJob(id, ts)),
				"enqueueing migration job for subscription '%s'", id)
		}
		return catcher.Resolve()
	}
}

// Phase 2: Cleanup — remove secrets from MongoDB after migration is verified.

type webhookSecretCleanupJob struct {
	job.Base       `bson:"job_base" json:"job_base" yaml:"job_base"`
	SubscriptionID string `bson:"subscription_id" json:"subscription_id" yaml:"subscription_id"`

	env evergreen.Environment
}

func makeWebhookSecretCleanupJob() *webhookSecretCleanupJob {
	j := &webhookSecretCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    webhookSecretCleanupJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewWebhookSecretCleanupJob creates a job to remove the secret field from MongoDB for a migrated subscription.
func NewWebhookSecretCleanupJob(subscriptionID, ts string) amboy.Job {
	j := makeWebhookSecretCleanupJob()
	j.SubscriptionID = subscriptionID
	j.SetID(fmt.Sprintf("%s.%s.%s", webhookSecretCleanupJobName, subscriptionID, ts))
	j.SetScopes([]string{
		fmt.Sprintf("%s.%s", webhookSecretCleanupJobName, subscriptionID),
	})
	j.SetEnqueueAllScopes(true)
	return j
}

func (j *webhookSecretCleanupJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}
	if flags.WebhookSecretCleanupDisabled {
		return
	}

	// Query as a guard: ResultsNotFound means there is nothing to clean up (not
	// found, wrong type, not yet migrated, or already cleaned up).
	sub := &event.Subscription{}
	if err := db.FindOneQ(ctx, event.SubscriptionsCollection, db.Query(append(bson.D{{Key: "_id", Value: j.SubscriptionID}}, migratedWebhookQuery...)), sub); err != nil {
		if !adb.ResultsNotFound(err) {
			j.AddError(errors.Wrapf(err, "finding subscription '%s'", j.SubscriptionID))
		}
		return
	}

	webhookSub, ok := sub.Subscriber.Target.(*event.WebhookSubscriber)
	if !ok {
		return
	}

	if len(webhookSub.Secret) > 0 && webhookSub.SecretParameter != "" {
		if err := db.Update(ctx, event.SubscriptionsCollection,
			bson.D{{Key: "_id", Value: j.SubscriptionID}},
			bson.D{{Key: "$unset", Value: bson.D{{Key: "subscriber.target.secret", Value: ""}}}},
		); err != nil {
			j.AddError(errors.Wrapf(err, "removing secret from MongoDB for subscription '%s'", j.SubscriptionID))
			return
		}
		grip.Info(ctx, message.Fields{
			"message":         "removed webhook secret from MongoDB",
			"subscription_id": j.SubscriptionID,
			"job_id":          j.ID(),
		})
	}

	if authValue := webhookSub.GetHeader("Authorization"); authValue != "" && webhookSub.AuthorizationHeaderParameter != "" {
		if err := db.Update(ctx, event.SubscriptionsCollection,
			bson.D{{Key: "_id", Value: j.SubscriptionID}},
			bson.D{{Key: "$pull", Value: bson.D{{Key: "subscriber.target.headers", Value: bson.D{{Key: "key", Value: "Authorization"}}}}}},
		); err != nil {
			j.AddError(errors.Wrapf(err, "removing Authorization header from MongoDB for subscription '%s'", j.SubscriptionID))
			return
		}
		grip.Info(ctx, message.Fields{
			"message":         "removed webhook Authorization header from MongoDB",
			"subscription_id": j.SubscriptionID,
			"job_id":          j.ID(),
		})
	}
}

// migratedWebhookQuery matches webhook subscriptions that have had their secret
// or Authorization header migrated to Parameter Store but the raw value is still
// present in MongoDB and needs cleanup.
var migratedWebhookQuery = bson.D{
	{Key: "subscriber.type", Value: event.EvergreenWebhookSubscriberType},
	{Key: "$or", Value: bson.A{
		bson.D{
			{Key: "subscriber.target.secret", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$ne", Value: nil}}},
			{Key: "subscriber.target.secret_parameter", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$ne", Value: ""}}},
		},
		bson.D{
			{Key: "subscriber.target.headers", Value: bson.D{{Key: "$elemMatch", Value: bson.D{
				{Key: "key", Value: "Authorization"},
				{Key: "value", Value: bson.D{{Key: "$ne", Value: ""}}},
			}}}},
			{Key: "subscriber.target.authorization_header_parameter", Value: bson.D{{Key: "$exists", Value: true}, {Key: "$ne", Value: ""}}},
		},
	}},
}

// findMigratedWebhookSubscriptionIDs returns IDs of webhook subscriptions that
// have been migrated to Parameter Store but still have raw secrets or Authorization
// headers in MongoDB.
func findMigratedWebhookSubscriptionIDs(ctx context.Context, limit int) ([]string, error) {
	subscriptions := []event.Subscription{}
	if err := db.FindAllQ(ctx, event.SubscriptionsCollection, db.Query(migratedWebhookQuery).Limit(limit), &subscriptions); err != nil {
		return nil, errors.Wrap(err, "finding migrated webhook subscriptions with raw credentials")
	}

	ids := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		ids = append(ids, sub.ID)
	}
	return ids, nil
}

// PopulateWebhookSecretCleanupJobs returns a QueueOperation that enqueues jobs
// to remove secrets from MongoDB for migrated subscriptions.
// Add to crons_remote_five_minute.go ops list when migration is verified complete.
func PopulateWebhookSecretCleanupJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.WebhookSecretCleanupDisabled {
			return nil
		}

		ids, err := findMigratedWebhookSubscriptionIDs(ctx, webhookSecretCleanupBatchSize)
		if err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}

		grip.Info(ctx, message.Fields{
			"message":     "enqueuing webhook secret cleanup jobs",
			"batch_count": len(ids),
		})

		ts := utility.RoundPartOfHour(5).Format(TSFormat)
		catcher := grip.NewBasicCatcher()
		for _, id := range ids {
			catcher.Wrapf(amboy.EnqueueUniqueJob(ctx, queue, NewWebhookSecretCleanupJob(id, ts)),
				"enqueueing cleanup job for subscription '%s'", id)
		}
		return catcher.Resolve()
	}
}
