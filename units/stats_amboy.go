package units

import (
	"errors"

	"github.com/evergreen-ci/sink/evergreen"
	"github.com/mongodb/amboy"
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
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
	logger   grip.Journaler
}

// NewAmboyStatsCollector reports the status of the local and remote
// queues registered in the evergreen service Environment.
func NewAmboyStatsCollector(env evergreen.Environment) amboy.Job {
	j := makeAmboyStatsCollector()
	j.env = env
	return j
}

func makeAmboyStatsCollector() *amboyStatsCollector {
	return &amboyStatsCollector{
		env:    evergreen.GetEnvironment(),
		logger: logging.MakeGrip(grip.GetSender()),
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    amboyStatsCollectorJobName,
				Version: 0,
				Format:  amboy.BSON,
			},
		},
	}
}

func (j *amboyStatsCollector) Run() {
	defer j.MarkComplete()

	if j.env == nil {
		j.AddError(errors.New("environment is not configured"))
		return
	}

	if localQueue = env.LocalQueue(); localQueue.Started() {
		j.logging.Info(message.Fields{
			"message": "amboy local queue stats",
			"stats":   localQueue.Stats(),
			"report":  amboy.Report(ctx, localQueue, numReportJobs),
		})
	}

	if remoteQueue = env.RemoteQueue(); remoteQueue.Started() {
		j.logging.Info(message.Fields{
			"message": "amboy remote queue stats",
			"stats":   remoteQueue.Stats(),
			"report":  amboy.Report(ctx, remoteQueue, numReportJobs),
		})
	}
}
