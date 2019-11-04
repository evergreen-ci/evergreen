package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/amboy/reporting"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
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
	if !j.ExcludeLocal && (localQueue != nil && localQueue.Started()) {
		j.logger.Info(message.Fields{
			"message": "amboy local queue stats",
			"stats":   localQueue.Stats(ctx),
		})
	}

	remoteQueue := j.env.RemoteQueue()
	if !j.ExcludeRemote && (remoteQueue != nil && remoteQueue.Started()) {
		j.logger.Info(message.Fields{
			"message": "amboy remote queue stats",
			"stats":   remoteQueue.Stats(ctx),
		})

		if evergreen.EnableAmboyRemoteReporting {
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

	reporter, err := reporting.MakeDBQueueState(ctx, reporting.DBQueueReporterOptions{
		Name:    settings.Amboy.Name,
		Options: opts,
	}, j.env.Client())
	if err != nil {
		j.AddError(err)
		return
	}

	r, err := buildAmboyQueueMessage("amboy remote queue report", reporter)
	j.AddError(err)
	j.logger.InfoWhen(len(r) > 1, r)
	return nil

}

func (j *amboyStatsCollector) collectExtendedGroupRemoteStats(ctx context.Context) {
	settings := j.env.Settings()
	opts := queue.DefaultMongoDBOptions()
	opts.URI = settings.Database.Url
	opts.DB = settings.Amboy.DB
	opts.Priority = true

	reporter, err := reporting.MakeDBQueueState(ctx, reporting.DBQueueReporterOptions{
		Name:     settings.Amboy.Name,
		Options:  opts,
		ByGroups: true,
	}, j.env.Client())
	if err != nil {
		j.AddError(err)
		return
	}

	r, err := buildAmboyQueueMessage("amboy remote queue group", reporter)
	j.AddError(err)
	j.logger.InfoWhen(len(r) > 1, r)
	return nil
}

func buildAmboyQueueMessage(msg string, reporter reporting.Reporter) (message.Fields, error) {
	catcher := grip.NewBasicCatcher()
	r := message.Fields{
		"message": msg,
	}

	pending, err := reporter.JobStatus(ctx, reporting.Pending)
	catcher.Add(err)
	if pending != nil {
		r["pending"] = pending
	}
	inprog, err := reporter.JobStatus(ctx, reporting.InProgress)
	catcher.Add(err)
	if inprog != nil {
		r["inprog"] = inprog
	}
	stale, err := reporter.JobStatus(ctx, reporting.Stale)
	catcher.Add(err)
	if stale != nil {
		r["stale"] = stale
	}

	recentErrors, err := reporter.RecentErrors(ctx, time.Minute, reporting.StatsOnly)
	catcher.Add(err)
	if recentErrors != nil {
		r["errors"] = recentErrors
	}

	return r, catcher.Resolve()
}
