package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
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
	if !j.ExcludeLocal && (localQueue != nil && localQueue.Info().Started) {
		j.logger.Info(message.Fields{
			"message": "amboy local queue stats",
			"stats":   localQueue.Stats(ctx),
		})
	}

	if !j.ExcludeRemote {
		remoteQueue := j.env.RemoteQueue()
		if remoteQueue != nil && remoteQueue.Info().Started {
			j.logger.Info(message.Fields{
				"message": "amboy remote queue stats",
				"stats":   remoteQueue.Stats(ctx),
			})
		}

		appCtx, _ := j.env.Context()
		queueGroup, _ := j.env.RemoteQueueGroup().Get(appCtx, terminateHostQueueGroup)
		if queueGroup != nil && queueGroup.Info().Started {
			j.logger.Info(message.Fields{
				"message": fmt.Sprintf("amboy '%s' group stats", terminateHostQueueGroup),
				"stats":   queueGroup.Stats(ctx),
			})
		}
	}

}
