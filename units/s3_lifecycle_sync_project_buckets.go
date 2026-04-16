package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/s3lifecycle"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const s3LifecycleSyncProjectBucketsJobName = "s3-lifecycle-sync-project-buckets"

func init() {
	registry.AddJobType(s3LifecycleSyncProjectBucketsJobName,
		func() amboy.Job { return makeS3LifecycleSyncProjectBucketsJob() })
}

type s3LifecycleSyncProjectBucketsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeS3LifecycleSyncProjectBucketsJob() *s3LifecycleSyncProjectBucketsJob {
	j := &s3LifecycleSyncProjectBucketsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    s3LifecycleSyncProjectBucketsJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewS3LifecycleSyncProjectBucketsJob creates a job to sync lifecycle rules for project buckets.
func NewS3LifecycleSyncProjectBucketsJob(id string) amboy.Job {
	j := makeS3LifecycleSyncProjectBucketsJob()
	j.SetID(fmt.Sprintf("%s.%s", s3LifecycleSyncProjectBucketsJobName, id))
	return j
}

func (j *s3LifecycleSyncProjectBucketsJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting service flags"))
		return
	}
	if flags.S3LifecycleSyncDisabled {
		grip.Info(ctx, message.Fields{
			"message": "S3 lifecycle sync is disabled, skipping project buckets sync",
			"job_id":  j.ID(),
		})
		return
	}

	// Get distinct list of user-specified buckets from collection.
	bucketNames, err := s3lifecycle.FindDistinctBucketNames(ctx, s3lifecycle.BucketTypeUserSpecified)
	if err != nil {
		j.AddError(errors.Wrap(err, "finding distinct bucket names"))
		return
	}

	if len(bucketNames) == 0 {
		grip.Info(ctx, message.Fields{
			"message": "no project buckets found to sync",
			"job_id":  j.ID(),
		})
		return
	}

	client := cloud.NewS3LifecycleClient()

	failureCount := 0
	var failedBuckets []string

	for _, bucketName := range bucketNames {
		if err := j.syncBucket(ctx, client, bucketName); err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message": "failed to sync bucket",
				"bucket":  bucketName,
				"job_id":  j.ID(),
			}))
			j.AddError(err)
			failureCount++
			failedBuckets = append(failedBuckets, bucketName)
			continue
		}
	}

	msg := message.Fields{
		"message":       "completed S3 lifecycle sync for project buckets",
		"success_count": len(bucketNames) - failureCount,
		"failure_count": failureCount,
		"total_buckets": len(bucketNames),
		"job_id":        j.ID(),
	}
	if len(failedBuckets) > 0 {
		msg["failed_buckets"] = failedBuckets
	}
	grip.Info(ctx, msg)
}

func (j *s3LifecycleSyncProjectBucketsJob) syncBucket(ctx context.Context, client cloud.S3LifecycleClient, bucketName string) error {
	// Get existing rules to extract region (all rules for same bucket have same region).
	existingRules, err := s3lifecycle.FindAllRulesForBucket(ctx, bucketName)
	if err != nil {
		return errors.Wrapf(err, "finding existing rules for bucket '%s'", bucketName)
	}

	if len(existingRules) == 0 {
		grip.Info(ctx, message.Fields{
			"message": "no existing rules found for bucket, skipping sync",
			"bucket":  bucketName,
			"job_id":  j.ID(),
		})
		return nil
	}

	region := existingRules[0].Region
	if region == "" {
		return errors.Errorf("region not set for bucket '%s'", bucketName)
	}

	// Fetch lifecycle configuration from AWS (returns array of rules).
	awsRules, err := client.GetBucketLifecycleConfiguration(ctx, bucketName, region, nil, nil)
	if err != nil {
		// Update all existing rules with sync error.
		var failedPrefixes []string
		var lastUpdateErr error
		for _, rule := range existingRules {
			if updateErr := s3lifecycle.UpdateSyncError(ctx, bucketName, rule.FilterPrefix, err.Error()); updateErr != nil {
				failedPrefixes = append(failedPrefixes, rule.FilterPrefix)
				lastUpdateErr = updateErr
			}
		}
		if len(failedPrefixes) > 0 {
			grip.Warning(ctx, message.WrapError(lastUpdateErr, message.Fields{
				"message":         "failed to update sync error for rules",
				"bucket":          bucketName,
				"failed_prefixes": failedPrefixes,
				"job_id":          j.ID(),
			}))
		}
		return errors.Wrapf(err, "fetching lifecycle configuration for bucket '%s'", bucketName)
	}

	// Upsert each AWS rule as separate document.
	awsPrefixes := make(map[string]bool)
	for _, awsRule := range awsRules {
		awsPrefixes[awsRule.Prefix] = true

		doc := convertPailRuleToDoc(bucketName, region, awsRule, existingRules)

		if err := doc.Upsert(ctx); err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message": "failed to upsert lifecycle rule",
				"bucket":  bucketName,
				"prefix":  awsRule.Prefix,
				"rule_id": awsRule.ID,
				"job_id":  j.ID(),
			}))
			j.AddError(err)
			continue
		}

		grip.Debug(ctx, message.Fields{
			"message": "upserted lifecycle rule",
			"bucket":  bucketName,
			"prefix":  awsRule.Prefix,
			"rule_id": awsRule.ID,
			"status":  awsRule.Status,
			"job_id":  j.ID(),
		})
	}

	// Remove obsolete rules that no longer exist in AWS.
	for _, existingRule := range existingRules {
		if !awsPrefixes[existingRule.FilterPrefix] {
			grip.Info(ctx, message.Fields{
				"message": "removing obsolete lifecycle rule",
				"bucket":  bucketName,
				"prefix":  existingRule.FilterPrefix,
				"job_id":  j.ID(),
			})

			if err := s3lifecycle.Remove(ctx, bucketName, existingRule.FilterPrefix); err != nil {
				grip.Warning(ctx, message.WrapError(err, message.Fields{
					"message": "failed to remove obsolete rule",
					"bucket":  bucketName,
					"prefix":  existingRule.FilterPrefix,
					"job_id":  j.ID(),
				}))
				j.AddError(err)
			}
		}
	}

	grip.Info(ctx, message.Fields{
		"message":           "successfully synced lifecycle rules for project bucket",
		"bucket":            bucketName,
		"num_rules_synced":  len(awsRules),
		"num_rules_removed": len(existingRules) - len(awsPrefixes),
		"job_id":            j.ID(),
	})

	return nil
}

// convertPailRuleToDoc converts a pail.LifecycleRule to S3LifecycleRuleDoc.
// Preserves ProjectAssociations from existing rules if available.
func convertPailRuleToDoc(bucketName, region string, awsRule pail.LifecycleRule, existingRules []s3lifecycle.S3LifecycleRuleDoc) *s3lifecycle.S3LifecycleRuleDoc {
	doc := &s3lifecycle.S3LifecycleRuleDoc{
		BucketName:              bucketName,
		FilterPrefix:            awsRule.Prefix,
		BucketType:              s3lifecycle.BucketTypeUserSpecified,
		Region:                  region,
		RuleID:                  awsRule.ID,
		RuleStatus:              awsRule.Status,
		ExpirationDays:          utility.ConvertInt32PtrToIntPtr(awsRule.ExpirationDays),
		TransitionToIADays:      utility.ConvertInt32PtrToIntPtr(awsRule.TransitionToIADays),
		TransitionToGlacierDays: utility.ConvertInt32PtrToIntPtr(awsRule.TransitionToGlacierDays),
		Transitions:             convertPailTransitions(awsRule.Transitions),
		LastSyncedAt:            time.Now(),
		SyncError:               "", // Clear any previous error on successful sync
	}

	// Preserve ProjectAssociations from existing rule if it exists.
	for _, existingRule := range existingRules {
		if existingRule.FilterPrefix == awsRule.Prefix {
			doc.ProjectAssociations = existingRule.ProjectAssociations
			break
		}
	}

	return doc
}

// convertPailTransitions converts pail.Transition to s3lifecycle.Transition.
func convertPailTransitions(pailTransitions []pail.Transition) []s3lifecycle.Transition {
	if len(pailTransitions) == 0 {
		return nil
	}

	transitions := make([]s3lifecycle.Transition, 0, len(pailTransitions))
	for _, pt := range pailTransitions {
		transitions = append(transitions, s3lifecycle.Transition{
			Days:         pt.Days,
			StorageClass: pt.StorageClass,
		})
	}

	return transitions
}

// PopulateS3LifecycleSyncProjectBucketsJob is a queue operation to populate the project buckets sync job.
func PopulateS3LifecycleSyncProjectBucketsJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.S3LifecycleSyncDisabled {
			return nil
		}

		ts := utility.RoundPartOfDay(0).Format(TSFormat)
		return queue.Put(ctx, NewS3LifecycleSyncProjectBucketsJob(ts))
	}
}
