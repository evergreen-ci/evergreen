package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/s3lifecycle"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	s3LifecycleSyncAdminBucketsJobName = "s3-lifecycle-sync-admin-buckets"
	lifecycleRuleStatusEnabled         = "Enabled"
)

func init() {
	registry.AddJobType(s3LifecycleSyncAdminBucketsJobName,
		func() amboy.Job { return makeS3LifecycleSyncAdminBucketsJob() })
}

type s3LifecycleSyncAdminBucketsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeS3LifecycleSyncAdminBucketsJob() *s3LifecycleSyncAdminBucketsJob {
	j := &s3LifecycleSyncAdminBucketsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    s3LifecycleSyncAdminBucketsJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewS3LifecycleSyncAdminBucketsJob creates a job to sync lifecycle rules for admin-managed buckets.
func NewS3LifecycleSyncAdminBucketsJob(id string) amboy.Job {
	j := makeS3LifecycleSyncAdminBucketsJob()
	j.SetID(fmt.Sprintf("%s.%s", s3LifecycleSyncAdminBucketsJobName, id))
	return j
}

func (j *s3LifecycleSyncAdminBucketsJob) Run(ctx context.Context) {
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
		grip.Info(message.Fields{
			"message": "S3 lifecycle sync is disabled, skipping admin buckets sync",
			"job_id":  j.ID(),
		})
		return
	}

	bucketsConfig := j.env.Settings().Buckets

	adminBuckets, err := s3lifecycle.DiscoverAdminManagedBuckets(ctx, j.env.Settings())
	if err != nil {
		j.AddError(errors.Wrap(err, "discovering admin-managed buckets"))
		return
	}

	grip.InfoWhen(len(adminBuckets) > 0, message.Fields{
		"message":     "starting S3 lifecycle sync for admin buckets",
		"num_buckets": len(adminBuckets),
		"job_id":      j.ID(),
	})

	client := cloud.NewS3LifecycleClient()

	successCount := 0

	for bucketName, bucketInfo := range adminBuckets {
		grip.Info(message.Fields{
			"message": "syncing lifecycle rules for admin bucket",
			"bucket":  bucketName,
			"region":  bucketInfo.Region,
			"job_id":  j.ID(),
		})

		rules, err := client.GetBucketLifecycleConfiguration(ctx, bucketInfo.Name, bucketInfo.Region, bucketInfo.RoleARN)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to get lifecycle configuration for admin bucket",
				"bucket":  bucketName,
				"region":  bucketInfo.Region,
				"job_id":  j.ID(),
			}))
			bucketField := getBucketFieldForName(bucketsConfig, bucketName)
			if bucketField != "" {
				if updateErr := evergreen.UpdateBucketLifecycleError(ctx, bucketField, err.Error()); updateErr != nil {
					grip.Error(message.WrapError(updateErr, message.Fields{
						"message":      "failed to update bucket lifecycle error",
						"bucket":       bucketName,
						"bucket_field": bucketField,
						"job_id":       j.ID(),
					}))
					j.AddError(updateErr)
				}
			}

			j.AddError(err)
			continue
		}

		// Extract lifecycle values from first rule (admin buckets typically have one rule).
		var expirationDays, transitionToIADays, transitionToGlacierDays *int
		if len(rules) > 0 {
			// Use the first enabled rule.
			for _, rule := range rules {
				if rule.Status != lifecycleRuleStatusEnabled {
					continue
				}

				expirationDays = utility.ConvertInt32PtrToIntPtr(rule.ExpirationDays)
				transitionToIADays = utility.ConvertInt32PtrToIntPtr(rule.TransitionToIADays)
				transitionToGlacierDays = utility.ConvertInt32PtrToIntPtr(rule.TransitionToGlacierDays)
				break
			}
		}

		bucketField := getBucketFieldForName(bucketsConfig, bucketName)
		if bucketField == "" {
			err := errors.Errorf("no bucket field found for bucket '%s'", bucketName)
			grip.Error(message.Fields{
				"message": "failed to find bucket field for admin bucket",
				"bucket":  bucketName,
				"job_id":  j.ID(),
			})
			j.AddError(err)
			continue
		}

		if err := evergreen.UpdateBucketLifecycle(ctx, bucketField, expirationDays, transitionToIADays, transitionToGlacierDays); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":      "failed to update bucket lifecycle configuration",
				"bucket":       bucketName,
				"bucket_field": bucketField,
				"job_id":       j.ID(),
			}))
			j.AddError(err)
			continue
		}

		successCount++
		grip.Info(message.Fields{
			"message":                    "successfully synced lifecycle rules for admin bucket",
			"bucket":                     bucketName,
			"expiration_days":            expirationDays,
			"transition_to_ia_days":      transitionToIADays,
			"transition_to_glacier_days": transitionToGlacierDays,
			"num_rules":                  len(rules),
			"job_id":                     j.ID(),
		})
	}

	grip.Info(message.Fields{
		"message":       "completed S3 lifecycle sync for admin buckets",
		"success_count": successCount,
		"failure_count": len(adminBuckets) - successCount,
		"total_buckets": len(adminBuckets),
		"job_id":        j.ID(),
	})
}

func getBucketFieldForName(bucketsConfig evergreen.BucketsConfig, bucketName string) string {
	if bucketsConfig.LogBucket.Name == bucketName {
		return evergreen.BucketsConfigLogBucketKey
	}
	if bucketsConfig.LogBucketLongRetention.Name == bucketName {
		return evergreen.BucketsConfigLogBucketLongRetentionKey
	}
	if bucketsConfig.LogBucketFailedTasks.Name == bucketName {
		return evergreen.BucketsConfigLogBucketFailedTasksKey
	}
	return ""
}

// PopulateS3LifecycleSyncAdminBucketsJob is a queue operation to populate the admin buckets sync job.
func PopulateS3LifecycleSyncAdminBucketsJob() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if flags.S3LifecycleSyncDisabled {
			return nil
		}

		ts := utility.RoundPartOfDay(0).Format(TSFormat)
		return queue.Put(ctx, NewS3LifecycleSyncAdminBucketsJob(ts))
	}
}
