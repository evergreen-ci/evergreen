package evergreen

import (
	"context"
	"regexp"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AmboyConfig struct {
	Name                                  string                  `bson:"name" json:"name" yaml:"name"`
	SingleName                            string                  `bson:"single_name" json:"single_name" yaml:"single_name"`
	DBConnection                          AmboyDBConfig           `bson:"db_connection" json:"db_connection" yaml:"db_connection"`
	PoolSizeLocal                         int                     `bson:"pool_size_local" json:"pool_size_local" yaml:"pool_size_local"`
	PoolSizeRemote                        int                     `bson:"pool_size_remote" json:"pool_size_remote" yaml:"pool_size_remote"`
	LocalStorage                          int                     `bson:"local_storage_size" json:"local_storage_size" yaml:"local_storage_size"`
	GroupDefaultWorkers                   int                     `bson:"group_default_workers" json:"group_default_workers" yaml:"group_default_workers"`
	GroupBackgroundCreateFrequencyMinutes int                     `bson:"group_background_create_frequency" json:"group_background_create_frequency" yaml:"group_background_create_frequency"`
	GroupPruneFrequencyMinutes            int                     `bson:"group_prune_frequency" json:"group_prune_frequency" yaml:"group_prune_frequency"`
	GroupTTLMinutes                       int                     `bson:"group_ttl" json:"group_ttl" yaml:"group_ttl"`
	RequireRemotePriority                 bool                    `bson:"require_remote_priority" json:"require_remote_priority" yaml:"require_remote_priority"`
	LockTimeoutMinutes                    int                     `bson:"lock_timeout_minutes" json:"lock_timeout_minutes" yaml:"lock_timeout_minutes"`
	SampleSize                            int                     `bson:"sample_size" json:"sample_size" yaml:"sample_size"`
	Retry                                 AmboyRetryConfig        `bson:"retry" json:"retry" yaml:"retry"`
	NamedQueues                           []AmboyNamedQueueConfig `bson:"named_queues" json:"named_queues" yaml:"named_queues"`
	// SkipPreferredIndexes indicates whether or not to use the preferred
	// indexes for the remote queues. This is not a value that can or should be
	// configured in production, but is useful to explicitly set for testing
	// environments, where the required indexes may not be set up.
	SkipPreferredIndexes bool `bson:"skip_preferred_indexes" json:"skip_preferred_indexes" yaml:"skip_preferred_indexes"`
}

// AmboyDBConfig configures Amboy's database connection.
type AmboyDBConfig struct {
	URL      string `bson:"url" json:"url" yaml:"url"`
	Database string `bson:"database" json:"database" yaml:"database"`
}

// AmboyRetryConfig represents configuration settings for Amboy's retryability
// feature.
type AmboyRetryConfig struct {
	NumWorkers                          int `bson:"num_workers" json:"num_workers" yaml:"num_workers"`
	MaxCapacity                         int `bson:"max_capacity" json:"max_capacity" yaml:"max_capacity"`
	MaxRetryAttempts                    int `bson:"max_retry_attempts" json:"max_retry_attempts" yaml:"max_retry_attempts"`
	MaxRetryTimeSeconds                 int `bson:"max_retry_time_seconds" json:"max_retry_time_seconds" yaml:"max_retry_time_seconds"`
	RetryBackoffSeconds                 int `bson:"retry_backoff_seconds" json:"retry_backoff_seconds" yaml:"retry_backoff_seconds"`
	StaleRetryingMonitorIntervalSeconds int `bson:"stale_retrying_monitor_interval_seconds" json:"stale_retrying_monitor_interval_seconds" yaml:"stale_retrying_monitor_interval_seconds"`
}

// AmboyNamedQueueConfig represents configuration settings for particular named
// queues in the Amboy queue group.
type AmboyNamedQueueConfig struct {
	Name               string `bson:"name" json:"name" yaml:"name"`
	Regexp             string `bson:"regexp" json:"regexp" yaml:"regexp"`
	NumWorkers         int    `bson:"num_workers" json:"num_workers" yaml:"num_workers"`
	SampleSize         int    `bson:"sample_size" json:"sample_size" yaml:"sample_size"`
	LockTimeoutSeconds int    `bson:"lock_timeout_seconds" json:"lock_timeout_seconds" yaml:"lock_timeout_seconds"`
}

var (
	amboyNameKey                                  = bsonutil.MustHaveTag(AmboyConfig{}, "Name")
	amboySingleNameKey                            = bsonutil.MustHaveTag(AmboyConfig{}, "SingleName")
	amboyDBConnectionKey                          = bsonutil.MustHaveTag(AmboyConfig{}, "DBConnection")
	amboyPoolSizeLocalKey                         = bsonutil.MustHaveTag(AmboyConfig{}, "PoolSizeLocal")
	amboyPoolSizeRemoteKey                        = bsonutil.MustHaveTag(AmboyConfig{}, "PoolSizeRemote")
	amboyLocalStorageKey                          = bsonutil.MustHaveTag(AmboyConfig{}, "LocalStorage")
	amboyGroupDefaultWorkersKey                   = bsonutil.MustHaveTag(AmboyConfig{}, "GroupDefaultWorkers")
	amboyGroupBackgroundCreateFrequencyMinutesKey = bsonutil.MustHaveTag(AmboyConfig{}, "GroupBackgroundCreateFrequencyMinutes")
	amboyGroupPruneFrequencyMinutesKey            = bsonutil.MustHaveTag(AmboyConfig{}, "GroupPruneFrequencyMinutes")
	amboyGroupTTLMinutesKey                       = bsonutil.MustHaveTag(AmboyConfig{}, "GroupTTLMinutes")
	amboyRequireRemotePriorityKey                 = bsonutil.MustHaveTag(AmboyConfig{}, "RequireRemotePriority")
	amboyLockTimeoutMinutesKey                    = bsonutil.MustHaveTag(AmboyConfig{}, "LockTimeoutMinutes")
	amboySampleSizeKey                            = bsonutil.MustHaveTag(AmboyConfig{}, "SampleSize")
	amboyRetryKey                                 = bsonutil.MustHaveTag(AmboyConfig{}, "Retry")
	amboyNamedQueuesKey                           = bsonutil.MustHaveTag(AmboyConfig{}, "NamedQueues")
)

func (c *AmboyConfig) SectionId() string { return "amboy" }

func (c *AmboyConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	grip.Info(res.Err())
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = AmboyConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}
	return nil
}

func (c *AmboyConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			amboyNameKey:                                  c.Name,
			amboySingleNameKey:                            c.SingleName,
			amboyDBConnectionKey:                          c.DBConnection,
			amboyPoolSizeLocalKey:                         c.PoolSizeLocal,
			amboyPoolSizeRemoteKey:                        c.PoolSizeRemote,
			amboyLocalStorageKey:                          c.LocalStorage,
			amboyGroupDefaultWorkersKey:                   c.GroupDefaultWorkers,
			amboyGroupBackgroundCreateFrequencyMinutesKey: c.GroupBackgroundCreateFrequencyMinutes,
			amboyGroupPruneFrequencyMinutesKey:            c.GroupPruneFrequencyMinutes,
			amboyGroupTTLMinutesKey:                       c.GroupTTLMinutes,
			amboyRequireRemotePriorityKey:                 c.RequireRemotePriority,
			amboyLockTimeoutMinutesKey:                    c.LockTimeoutMinutes,
			amboySampleSizeKey:                            c.SampleSize,
			amboyRetryKey:                                 c.Retry,
			amboyNamedQueuesKey:                           c.NamedQueues,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

const (
	// DefaultAmboyQueueName is the default namespace prefix for the Amboy
	// remote queue.
	DefaultAmboyQueueName = "evg.service"

	defaultLogBufferingDuration                  = 20
	defaultLogBufferingCount                     = 100
	defaultLogBufferingIncomingFactor            = 10
	defaultAmboyPoolSize                         = 2
	defaultAmboyLocalStorageSize                 = 1024
	defaultSingleAmboyQueueName                  = "evg.single"
	defaultAmboyDBName                           = "amboy"
	defaultGroupWorkers                          = 1
	defaultGroupBackgroundCreateFrequencyMinutes = 10
	defaultGroupPruneFrequencyMinutes            = 10
	defaultGroupTTLMinutes                       = 1
	maxNotificationsPerSecond                    = 100
)

func (c *AmboyConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	for _, namedQueue := range c.NamedQueues {
		if namedQueue.Regexp != "" {
			_, err := regexp.Compile(namedQueue.Regexp)
			catcher.Wrapf(err, "invalid regexp '%s'", namedQueue.Regexp)
		}
	}
	if catcher.HasErrors() {
		return errors.Wrap(catcher.Resolve(), "invalid regexp for named queues")
	}

	if c.Name == "" {
		c.Name = DefaultAmboyQueueName
	}

	if c.SingleName == "" {
		c.SingleName = defaultSingleAmboyQueueName
	}

	if c.DBConnection.Database == "" {
		c.DBConnection.Database = defaultAmboyDBName
	}

	if c.PoolSizeLocal == 0 {
		c.PoolSizeLocal = defaultAmboyPoolSize
	}

	if c.PoolSizeRemote == 0 {
		c.PoolSizeRemote = defaultAmboyPoolSize
	}

	if c.LocalStorage == 0 {
		c.LocalStorage = defaultAmboyLocalStorageSize
	}

	if c.GroupDefaultWorkers <= 0 {
		c.GroupDefaultWorkers = defaultGroupWorkers
	}

	if c.GroupBackgroundCreateFrequencyMinutes <= 0 {
		c.GroupBackgroundCreateFrequencyMinutes = defaultGroupBackgroundCreateFrequencyMinutes
	}

	if c.GroupPruneFrequencyMinutes <= 0 {
		c.GroupPruneFrequencyMinutes = defaultGroupPruneFrequencyMinutes
	}

	if c.GroupTTLMinutes <= 0 {
		c.GroupTTLMinutes = defaultGroupTTLMinutes
	}
	if c.LockTimeoutMinutes <= 0 {
		c.LockTimeoutMinutes = int(amboy.LockTimeout / time.Minute)
	}

	return nil
}

func (c *AmboyRetryConfig) RetryableQueueOptions() queue.RetryableQueueOptions {
	return queue.RetryableQueueOptions{
		RetryHandler: amboy.RetryHandlerOptions{
			NumWorkers:       c.NumWorkers,
			MaxRetryAttempts: c.MaxRetryAttempts,
			MaxRetryTime:     time.Duration(c.MaxRetryTimeSeconds) * time.Second,
			RetryBackoff:     time.Duration(c.RetryBackoffSeconds) * time.Second,
			MaxCapacity:      c.MaxCapacity,
		},
		StaleRetryingMonitorInterval: time.Duration(c.StaleRetryingMonitorIntervalSeconds) * time.Second,
	}
}
