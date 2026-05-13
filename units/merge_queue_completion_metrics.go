package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const mergeQueueCompletionMetricsFallbackJobName = "merge-queue-completion-metrics-fallback"

func init() {
	registry.AddJobType(mergeQueueCompletionMetricsFallbackJobName, NewMergeQueueCompletionMetricsFallbackJob)
}

type mergeQueueCompletionMetricsFallbackJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewMergeQueueCompletionMetricsFallbackJob creates a job to emit completion metrics for merge queue patches that missed the GitHub destroyed webhook.
func NewMergeQueueCompletionMetricsFallbackJob() amboy.Job {
	j := &mergeQueueCompletionMetricsFallbackJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    mergeQueueCompletionMetricsFallbackJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", mergeQueueCompletionMetricsFallbackJobName, utility.RoundPartOfHour(5).Format(TSFormat)))
	return j
}

// Run finds finalized merge queue patches that missed the GitHub webhook, polls the GitHub PR API,
// and emits completion metrics for any that are confirmed merged.
func (j *mergeQueueCompletionMetricsFallbackJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	projectRefs, err := model.FindProjectRefsWithMergeQueueEnabled(ctx)
	if err != nil {
		grip.Debug(ctx, message.WrapError(err, message.Fields{
			"message": "error finding projects with merge queue enabled",
			"job":     j.ID(),
		}))
		j.AddError(errors.Wrap(err, "finding projects with merge queue enabled"))
		return
	}

	for _, projectRef := range projectRefs {
		patches, err := patch.FindFinalizedMergeQueuePatchesMissingCompletionMetrics(ctx, projectRef.Id)
		if err != nil {
			grip.Debug(ctx, message.WrapError(err, message.Fields{
				"message":    "error querying merge queue patches missing completion metrics",
				"project_id": projectRef.Id,
				"job":        j.ID(),
			}))
			j.AddError(err)
			continue
		}
		for i := range patches {
			j.emitCompletionMetricsForPatch(ctx, &patches[i])
		}
	}
}

func (j *mergeQueueCompletionMetricsFallbackJob) emitCompletionMetricsForPatch(ctx context.Context, p *patch.Patch) {
	_, collectiveFinishTime, err := p.GetCollectiveTimes(ctx)
	if err != nil {
		grip.Debug(ctx, message.WrapError(err, message.Fields{
			"message":  "could not get collective times for merge queue patch",
			"patch_id": p.Id.Hex(),
			"job":      j.ID(),
		}))
		return
	}
	// Wait 5 minutes to allow the PR status time to settle after CI and give the destroyed webhook a chance to arrive.
	if collectiveFinishTime.IsZero() || time.Since(collectiveFinishTime) < 5*time.Minute {
		return
	}

	pr, err := p.GithubMergeData.GetPullRequest(ctx)
	if err != nil {
		grip.Debug(ctx, message.WrapError(err, message.Fields{
			"message":  "could not get GitHub pull request for merge queue patch",
			"patch_id": p.Id.Hex(),
			"job":      j.ID(),
		}))
		return
	}

	endTime, endTimeSource, ok := mergeQueueEndTimeFromPR(pr, collectiveFinishTime)
	if !ok {
		return
	}

	claimed, err := patch.ClaimMergeQueueMetricsEmit(ctx, p.Id)
	if err != nil || !claimed {
		return
	}

	v, err := model.VersionFindOneId(ctx, p.Version)
	if err != nil || v == nil {
		_ = patch.SetMergeQueueMetricsEmitStatus(ctx, p.Id, patch.MergeQueueMetricsEmitStatusFailed)
		return
	}

	if err := model.EmitMergeQueueCompletionMetrics(ctx, p, v, p.Status, endTime, endTimeSource); err != nil {
		_ = patch.SetMergeQueueMetricsEmitStatus(ctx, p.Id, patch.MergeQueueMetricsEmitStatusFailed)
		grip.Debug(ctx, message.WrapError(err, message.Fields{
			"message":  "could not emit completion metrics for merge queue patch",
			"patch_id": p.Id.Hex(),
			"job":      j.ID(),
		}))
		return
	}
	if err := patch.SetRemovedFromQueueAt(ctx, p.Id, endTime); err != nil {
		grip.Debug(ctx, message.WrapError(err, message.Fields{
			"message":  "could not set removed_from_queue_at after cron emission",
			"patch_id": p.Id.Hex(),
			"job":      j.ID(),
		}))
	}
}

// mergeQueueEndTimeFromPR determines the end time and source for a merge queue patch based on the
// current GitHub PR state. Returns ok=false if the PR is still open and not in draft, meaning we
// should skip and retry on the next cron run.
func mergeQueueEndTimeFromPR(pr *github.PullRequest, collectiveFinishTime time.Time) (endTime time.Time, endTimeSource string, ok bool) {
	if pr.GetMerged() {
		return pr.GetMergedAt().Time, patch.MergeQueueEndTimeSourceGitHubPolling, true
	}

	if pr.GetState() == "closed" {
		// Once GitHub closes the PR, its time in the merge queue is done regardless of
		// when Evergreen tasks finished.
		closedAt := pr.GetClosedAt().Time
		if !closedAt.IsZero() {
			return closedAt, patch.MergeQueueEndTimeSourceGitHubPolling, true
		}
		return collectiveFinishTime, patch.MergeQueueEndTimeSourceCollectiveFinish, true
	}

	if pr.GetDraft() {
		// PR was converted to draft, removing it from the merge queue. No GitHub close time
		// is available, so fall back to Evergreen's collective finish time.
		return collectiveFinishTime, patch.MergeQueueEndTimeSourceCollectiveFinish, true
	}

	// PR is still open and active — may still be in the queue. Skip and retry next run.
	return time.Time{}, "", false
}

// PopulateMergeQueueCompletionMetricsFallbackJobs enqueues a job to emit completion metrics for
// merge queue patches that missed the GitHub webhook.
func PopulateMergeQueueCompletionMetricsFallbackJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.MonitorDisabled {
			return nil
		}

		j := NewMergeQueueCompletionMetricsFallbackJob()
		return queue.Put(ctx, j)
	}
}
