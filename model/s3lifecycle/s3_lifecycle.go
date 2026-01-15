package s3lifecycle

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/pail"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	Collection = "s3_lifecycle_rules"
)

const (
	BucketTypeAdminManaged  = "admin_managed"
	BucketTypeUserSpecified = "user_specified"
)

const (
	AdminBucketCategoryLog              = "log"
	AdminBucketCategoryLogLongRetention = "log_long_retention"
	AdminBucketCategoryLogFailedTasks   = "log_failed_tasks"
)

var (
	IDKey                      = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "ID")
	BucketNameKey              = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "BucketName")
	FilterPrefixKey            = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "FilterPrefix")
	BucketTypeKey              = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "BucketType")
	RegionKey                  = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "Region")
	AdminBucketCategoryKey     = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "AdminBucketCategory")
	ProjectAssociationsKey     = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "ProjectAssociations")
	RuleIDKey                  = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "RuleID")
	RuleStatusKeyDoc           = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "RuleStatus")
	ExpirationDaysKey          = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "ExpirationDays")
	TransitionToIADaysKey      = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "TransitionToIADays")
	TransitionToGlacierDaysKey = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "TransitionToGlacierDays")
	TransitionsKey             = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "Transitions")
	LastSyncedAtKey            = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "LastSyncedAt")
	SyncErrorKey               = bsonutil.MustHaveTag(S3LifecycleRuleDoc{}, "SyncError")
)

// BucketInfo contains information needed to fetch lifecycle rules for a bucket.
type BucketInfo struct {
	Name    string
	Region  string
	RoleARN *string
}

// DiscoverAdminManagedBuckets returns information about admin-managed buckets from BucketsConfig.
func DiscoverAdminManagedBuckets(ctx context.Context, settings *evergreen.Settings) (map[string]BucketInfo, error) {
	buckets := make(map[string]BucketInfo)
	bucketsConfig := settings.Buckets

	// Admin-managed buckets
	adminBuckets := []evergreen.BucketConfig{
		bucketsConfig.LogBucket,
		bucketsConfig.LogBucketLongRetention,
		bucketsConfig.LogBucketFailedTasks,
	}

	for _, bucket := range adminBuckets {
		if bucket.Type != evergreen.BucketTypeS3 || bucket.Name == "" {
			continue
		}

		info := BucketInfo{
			Name:   bucket.Name,
			Region: evergreen.DefaultS3Region,
		}

		if bucket.RoleARN != "" {
			arn := bucket.RoleARN
			info.RoleARN = &arn
		}

		buckets[bucket.Name] = info
	}

	if len(buckets) == 0 {
		return buckets, errors.New("no admin-managed S3 buckets found in config")
	}

	return buckets, nil
}

// S3LifecycleRuleDoc represents a single S3 lifecycle rule for a bucket+prefix combination.
// Each rule is stored as a separate document with a composite ID "bucketName#filterPrefix".
type S3LifecycleRuleDoc struct {
	ID string `bson:"_id" json:"id"`

	BucketName   string `bson:"bucket_name" json:"bucket_name"`
	FilterPrefix string `bson:"filter_prefix" json:"filter_prefix"` // Empty string applies to all objects (default rule)
	BucketType   string `bson:"bucket_type" json:"bucket_type"`
	Region       string `bson:"region" json:"region"`

	AdminBucketCategory string   `bson:"admin_bucket_category,omitempty" json:"admin_bucket_category,omitempty"`
	ProjectAssociations []string `bson:"project_associations" json:"project_associations"`

	RuleID     string `bson:"rule_id" json:"rule_id"`
	RuleStatus string `bson:"rule_status" json:"rule_status"`

	// Lifecycle configuration fields for cost calculation.
	// Note: These use *int (not *int32) for consistency with existing Evergreen DB schemas.
	// Pail provides these as *int32, so conversion is needed during sync.
	ExpirationDays          *int         `bson:"expiration_days,omitempty" json:"expiration_days,omitempty"`
	TransitionToIADays      *int         `bson:"transition_to_ia_days,omitempty" json:"transition_to_ia_days,omitempty"`
	TransitionToGlacierDays *int         `bson:"transition_to_glacier_days,omitempty" json:"transition_to_glacier_days,omitempty"`
	Transitions             []Transition `bson:"transitions,omitempty" json:"transitions,omitempty"`

	LastSyncedAt time.Time `bson:"last_synced_at" json:"last_synced_at"`
	SyncError    string    `bson:"sync_error,omitempty" json:"sync_error,omitempty"`
}

// Transition specifies when objects transition to a different storage class.
type Transition struct {
	Days         *int32 `bson:"days,omitempty" json:"days,omitempty"`
	StorageClass string `bson:"storage_class" json:"storage_class"`
}

func (s *S3LifecycleRuleDoc) Upsert(ctx context.Context) error {
	if s.BucketName == "" {
		return errors.New("bucket name cannot be empty")
	}

	s.ID = makeRuleID(s.BucketName, s.FilterPrefix)

	update := bson.M{
		BucketNameKey:              s.BucketName,
		FilterPrefixKey:            s.FilterPrefix,
		BucketTypeKey:              s.BucketType,
		RegionKey:                  s.Region,
		AdminBucketCategoryKey:     s.AdminBucketCategory,
		ProjectAssociationsKey:     s.ProjectAssociations,
		RuleIDKey:                  s.RuleID,
		RuleStatusKeyDoc:           s.RuleStatus,
		ExpirationDaysKey:          s.ExpirationDays,
		TransitionToIADaysKey:      s.TransitionToIADays,
		TransitionToGlacierDaysKey: s.TransitionToGlacierDays,
		TransitionsKey:             s.Transitions,
		LastSyncedAtKey:            s.LastSyncedAt,
		SyncErrorKey:               s.SyncError,
	}

	_, err := db.Upsert(
		ctx,
		Collection,
		bson.M{"_id": s.ID},
		bson.M{"$set": update},
	)

	return errors.Wrapf(err, "upserting lifecycle rule for bucket '%s' prefix '%s'", s.BucketName, s.FilterPrefix)
}

func makeRuleID(bucketName, filterPrefix string) string {
	return fmt.Sprintf("%s#%s", bucketName, filterPrefix)
}

// FindByBucketAndPrefix retrieves a single lifecycle rule document by bucket name and prefix.
func FindByBucketAndPrefix(ctx context.Context, bucketName, filterPrefix string) (*S3LifecycleRuleDoc, error) {
	if bucketName == "" {
		return nil, errors.New("bucket name cannot be empty")
	}

	docID := makeRuleID(bucketName, filterPrefix)
	doc := &S3LifecycleRuleDoc{}
	err := db.FindOneQ(ctx, Collection, db.Query(bson.M{"_id": docID}), doc)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return doc, errors.Wrapf(err, "finding lifecycle rule for bucket '%s' prefix '%s'", bucketName, filterPrefix)
}

// FindMatchingRuleForFileKey finds the most specific enabled lifecycle rule using longest-prefix matching.
// Tries progressively shorter prefixes until a match is found (O(depth) indexed queries).
func FindMatchingRuleForFileKey(ctx context.Context, bucketName, fileKey string) (*S3LifecycleRuleDoc, error) {
	if bucketName == "" {
		return nil, errors.New("bucket name cannot be empty")
	}
	if fileKey == "" {
		return nil, errors.New("file key cannot be empty")
	}

	prefixes := pail.ExtractPrefixHierarchy(fileKey)

	for _, prefix := range prefixes {
		doc, err := FindByBucketAndPrefix(ctx, bucketName, prefix)
		if err != nil {
			return nil, err
		}
		if doc != nil && doc.RuleStatus == "Enabled" {
			return doc, nil
		}
	}

	return nil, nil
}

func FindAllRulesForBucket(ctx context.Context, bucketName string) ([]S3LifecycleRuleDoc, error) {
	if bucketName == "" {
		return nil, errors.New("bucket name cannot be empty")
	}

	return findS3LifecycleRules(ctx, bson.M{BucketNameKey: bucketName})
}

func FindDistinctBucketNames(ctx context.Context, bucketType string) ([]string, error) {
	if bucketType == "" {
		return nil, errors.New("bucket type cannot be empty")
	}

	pipeline := []bson.M{
		{"$match": bson.M{BucketTypeKey: bucketType}},
		{"$group": bson.M{"_id": "$" + BucketNameKey}},
	}

	var results []struct {
		ID string `bson:"_id"`
	}

	err := db.Aggregate(ctx, Collection, pipeline, &results)
	if err != nil {
		return nil, errors.Wrapf(err, "finding distinct bucket names for bucket type '%s'", bucketType)
	}

	bucketNames := make([]string, 0, len(results))
	for _, result := range results {
		bucketNames = append(bucketNames, result.ID)
	}

	return bucketNames, nil
}

func findS3LifecycleRules(ctx context.Context, query bson.M) ([]S3LifecycleRuleDoc, error) {
	docs := []S3LifecycleRuleDoc{}
	return docs, db.FindAllQ(ctx, Collection, db.Query(query), &docs)
}

func Remove(ctx context.Context, bucketName, filterPrefix string) error {
	if bucketName == "" {
		return errors.New("bucket name cannot be empty")
	}

	docID := makeRuleID(bucketName, filterPrefix)
	return errors.Wrapf(
		db.Remove(ctx, Collection, bson.M{"_id": docID}),
		"removing lifecycle rule for bucket '%s' prefix '%s'",
		bucketName, filterPrefix,
	)
}

// RemoveAll removes all documents from the collection. Use with caution.
func RemoveAll(ctx context.Context) error {
	return errors.Wrap(
		db.RemoveAll(ctx, Collection, bson.M{}),
		"removing all lifecycle rule documents",
	)
}

// UpdateSyncError updates the sync error field. Pass an empty string to clear the error.
func UpdateSyncError(ctx context.Context, bucketName, filterPrefix, syncError string) error {
	if bucketName == "" {
		return errors.New("bucket name cannot be empty")
	}

	docID := makeRuleID(bucketName, filterPrefix)
	update := bson.M{
		"$set": bson.M{LastSyncedAtKey: time.Now()},
	}

	if syncError == "" {
		update["$unset"] = bson.M{SyncErrorKey: ""}
	} else {
		update["$set"].(bson.M)[SyncErrorKey] = syncError
	}

	return errors.Wrapf(
		db.UpdateId(ctx, Collection, docID, update),
		"updating sync error for bucket '%s' prefix '%s'",
		bucketName, filterPrefix,
	)
}
