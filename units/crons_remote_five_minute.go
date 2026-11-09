package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	cronsRemoteFiveMinuteJobName = "crons-remote-five-minute"

	// githubStatusResyncBatchSize caps how many patches have their GitHub
	// statuses resent per run, since each patch sends a status per build and
	// child patch.
	githubStatusResyncBatchSize = 20
)

func init() {
	registry.AddJobType(cronsRemoteFiveMinuteJobName, NewCronRemoteFiveMinuteJob)
}

type cronsRemoteFiveMinuteJob struct {
	job.Base   `bson:"job_base" json:"job_base" yaml:"job_base"`
	ErrorCount int `bson:"error_count" json:"error_count" yaml:"error_count"`
	env        evergreen.Environment
}

func NewCronRemoteFiveMinuteJob() amboy.Job {
	j := &cronsRemoteFiveMinuteJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cronsRemoteFiveMinuteJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", cronsRemoteFiveMinuteJobName, utility.RoundPartOfHour(5).Format(TSFormat)))
	return j
}

func (j *cronsRemoteFiveMinuteJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	ops := []amboy.QueueOperation{
		PopulateTaskMonitoring(5),
		PopulateActivationJobs(10),
		PopulateMergeQueuePatchRecoveryJobs(),
		PopulateHostProvisioningConversionJobs(j.env),
		PopulateHostRestartJasperJobs(j.env),
		PopulateGithubAPILimitJob(),
		PopulateMergeQueueMetricsJobs(),
		PopulateMergeQueueCompletionMetricsFallbackJobs(),
		PopulateLargeParserProjectTaskStatsJob(),
		PopulateGitHubStatusResyncJobs(),
	}

	queue := j.env.RemoteQueue()
	catcher := grip.NewBasicCatcher()
	for _, op := range ops {
		if ctx.Err() != nil {
			j.AddError(errors.New("operation aborted"))
		}

		catcher.Add(op(ctx, queue))
	}
	j.ErrorCount = catcher.Len()

	grip.Debug(ctx, message.Fields{
		"queue": "service",
		"id":    j.ID(),
		"type":  j.Type().Name,
		"num":   len(ops),
		"errs":  j.ErrorCount,
	})
}

// PopulateGitHubStatusResyncJobs enqueues jobs to resend the GitHub statuses of
// patches whose statuses failed to send.
func PopulateGitHubStatusResyncJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		jobs, err := githubStatusResyncJobs(ctx, utility.RoundPartOfHour(5))
		if err != nil {
			return err
		}
		return errors.Wrap(amboy.EnqueueManyUniqueJobs(ctx, queue, jobs), "enqueueing GitHub status resync jobs")
	}
}

func githubStatusResyncJobs(ctx context.Context, ts time.Time) ([]amboy.Job, error) {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting service flags")
	}
	// Statuses that can't be sent while the GitHub status API is disabled stay
	// marked so they're resent once it's turned back on.
	if flags.GithubStatusResyncDisabled || flags.GithubStatusAPIDisabled {
		return nil, nil
	}

	resyncs, err := patch.FindDueGitHubStatusResyncs(ctx, githubStatusResyncBatchSize)
	if err != nil {
		return nil, err
	}
	jobs := make([]amboy.Job, 0, len(resyncs))
	for _, resync := range resyncs {
		jobs = append(jobs, NewGithubStatusResyncRefreshJob(resync.PatchID, ts))
	}
	return jobs, nil
}
