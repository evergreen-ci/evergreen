package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/management"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const amboyStatsCollectorJobName = "amboy-stats-collector"

func init() {
	registry.AddJobType(amboyStatsCollectorJobName,
		func() amboy.Job { return makeAmboyStatsCollector() })
}

type amboyStatsCollector struct {
	ExcludeLocal  bool `bson:"exclude_local" json:"exclude_local" yaml:"exclude_local"`
	ExcludeRemote bool `bson:"exclude_remote" json:"exclude_remote" yaml:"exclude_remote"`
	job.Base      `bson:"job_base" json:"job_base" yaml:"job_base"`
	env           evergreen.Environment
	logger        grip.Journaler
}

// NewLocalAmboyStatsCollector reports the status of only the local queue
// registered in the evergreen service Environment.
func NewLocalAmboyStatsCollector(env evergreen.Environment, id string) amboy.Job {
	j := makeAmboyStatsCollector()
	j.ExcludeRemote = true
	j.env = env
	j.SetID(fmt.Sprintf("%s-%s", amboyStatsCollectorJobName, id))
	return j
}

// NewRemoteAmboyStatsCollector reports the status of only the remote queue
// registered in the evergreen service Environment.
func NewRemoteAmboyStatsCollector(env evergreen.Environment, id string) amboy.Job {
	j := makeAmboyStatsCollector()
	j.ExcludeLocal = true
	j.env = env
	j.SetID(fmt.Sprintf("%s-%s", amboyStatsCollectorJobName, id))
	return j
}

func makeAmboyStatsCollector() *amboyStatsCollector {
	j := &amboyStatsCollector{
		env:    evergreen.GetEnvironment(),
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    amboyStatsCollectorJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *amboyStatsCollector) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.logger == nil {
		j.logger = logging.MakeGrip(grip.GetSender())
	}

	localQueue := j.env.LocalQueue()
	if !j.ExcludeLocal && (localQueue != nil && localQueue.Info().Started) {
		j.logger.Info(message.Fields{
			"message": "amboy local queue stats",
			"stats":   localQueue.Stats(ctx),
		})
	}

	remoteQueue := j.env.RemoteQueue()
	if !j.ExcludeRemote && (remoteQueue != nil && remoteQueue.Info().Started) {
		j.logger.Info(message.Fields{
			"message": "amboy remote queue stats",
			"stats":   remoteQueue.Stats(ctx),
		})

		serviceFlags, err := evergreen.GetServiceFlags()
		if err != nil {
			j.AddError(errors.Wrap(err, "problem getting service flags"))
			return
		}
		if !serviceFlags.AmboyRemoteManagementDisabled {
			j.collectExtendedRemoteStats(ctx)
			j.collectExtendedGroupRemoteStats(ctx)
		}
	}
}

func (j *amboyStatsCollector) collectExtendedRemoteStats(ctx context.Context) {
	settings := j.env.Settings()

	opts := queue.DefaultMongoDBOptions()
	opts.URI = settings.Database.Url
	opts.DB = settings.Amboy.DB
	opts.Priority = true

	manager, err := management.MakeDBQueueManager(ctx, management.DBQueueManagerOptions{
		Name:    settings.Amboy.Name,
		Options: opts,
	}, j.env.Client())
	if err != nil {
		j.AddError(err)
		return
	}

	r, err := buildAmboyQueueMessage(ctx, "amboy remote queue report", manager)
	j.AddError(err)
	j.logger.InfoWhen(len(r) > 1, r)
}

func (j *amboyStatsCollector) collectExtendedGroupRemoteStats(ctx context.Context) {
	settings := j.env.Settings()
	opts := queue.DefaultMongoDBOptions()
	opts.URI = settings.Database.Url
	opts.DB = settings.Amboy.DB
	opts.Priority = true

	manager, err := management.MakeDBQueueManager(ctx, management.DBQueueManagerOptions{
		Name:     settings.Amboy.Name,
		Options:  opts,
		ByGroups: true,
	}, j.env.Client())
	if err != nil {
		j.AddError(err)
		return
	}

	r, err := buildAmboyQueueMessage(ctx, "amboy remote queue group", manager)
	j.AddError(err)
	j.logger.InfoWhen(len(r) > 1, r)
}

func buildAmboyQueueMessage(ctx context.Context, msg string, manager management.Manager) (message.Fields, error) {
	catcher := grip.NewBasicCatcher()
	r := message.Fields{
		"message": msg,
	}

	pending, err := manager.JobStatus(ctx, management.Pending)
	catcher.Add(err)
	if pending != nil {
		r["pending"] = pending
	}
	inprog, err := manager.JobStatus(ctx, management.InProgress)
	catcher.Add(err)
	if inprog != nil {
		r["inprog"] = inprog
	}
	stale, err := manager.JobStatus(ctx, management.Stale)
	catcher.Add(err)
	if stale != nil {
		r["stale"] = stale
	}

	return r, catcher.Resolve()
}
