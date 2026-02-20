package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const mergeQueueMetricsJobName = "merge-queue-metrics"

func init() {
	registry.AddJobType(mergeQueueMetricsJobName, NewMergeQueueMetricsJob)
}

type mergeQueueMetricsJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewMergeQueueMetricsJob creates a job to emit merge queue depth metrics.
func NewMergeQueueMetricsJob() amboy.Job {
	j := &mergeQueueMetricsJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    mergeQueueMetricsJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", mergeQueueMetricsJobName, utility.RoundPartOfHour(5).Format(TSFormat)))
	return j
}

// Run collects and emits merge queue depth metrics for all projects with merge queue enabled.
func (j *mergeQueueMetricsJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	projectRefs, err := model.FindProjectRefsWithMergeQueueEnabled(ctx)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error finding projects with merge queue enabled",
			"job_id":  j.ID(),
		}))
		j.AddError(errors.Wrap(err, "finding projects with merge queue enabled"))
		return
	}

	for _, projectRef := range projectRefs {
		if err := j.emitMetricsForProject(ctx, projectRef.Id); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "error emitting merge queue metrics for project",
				"project_id": projectRef.Id,
				"job":        j.ID(),
			}))
			j.AddError(err)
		}
	}
}

func (j *mergeQueueMetricsJob) emitMetricsForProject(ctx context.Context, projectID string) error {
	patches, err := patch.FindMergeQueuePatchesByProject(ctx, projectID)
	if err != nil {
		return errors.Wrapf(err, "querying merge queue patches for project '%s'", projectID)
	}

	if len(patches) == 0 {
		return nil
	}

	// Group patches by queue (org/repo/base_branch combination)
	type queueKey struct {
		org        string
		repo       string
		baseBranch string
	}
	queuePatches := make(map[queueKey][]patch.Patch)

	for i := range patches {
		p := patches[i]
		if p.GithubMergeData.Org == "" || p.GithubMergeData.Repo == "" || p.GithubMergeData.BaseBranch == "" {
			continue
		}
		key := queueKey{
			org:        p.GithubMergeData.Org,
			repo:       p.GithubMergeData.Repo,
			baseBranch: p.GithubMergeData.BaseBranch,
		}
		queuePatches[key] = append(queuePatches[key], p)
	}

	for key, queuePatchList := range queuePatches {
		if err := j.emitMetricsForQueue(ctx, projectID, key.org, key.repo, key.baseBranch, queuePatchList); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":     "error emitting metrics for queue",
				"project_id":  projectID,
				"org":         key.org,
				"repo":        key.repo,
				"base_branch": key.baseBranch,
			}))
			j.AddError(err)
			continue
		}
	}

	return nil
}

func (j *mergeQueueMetricsJob) emitMetricsForQueue(ctx context.Context, projectID, org, repo, baseBranch string, patches []patch.Patch) error {
	depth := int64(len(patches))
	pendingCount := int64(0)
	runningCount := int64(0)
	var oldestPatch *patch.Patch
	versionIDs := make([]string, 0, len(patches))

	for i := range patches {
		p := &patches[i]
		versionIDs = append(versionIDs, p.Id.Hex())

		if p.Status == evergreen.VersionCreated {
			pendingCount++
		} else if p.Status == evergreen.VersionStarted {
			runningCount++
		}

		// Track the patch with earliest CreateTime (oldest patch = top of queue).
		if oldestPatch == nil || p.CreateTime.Before(oldestPatch.CreateTime) {
			oldestPatch = p
		}
	}

	oldestPatchAgeMs := int64(0)
	if oldestPatch != nil {
		oldestPatchAgeMs = time.Since(oldestPatch.CreateTime).Milliseconds()
	}

	runningTasksCount, err := task.CountRunningTasksForVersions(ctx, versionIDs)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":     "error counting running tasks in merge queue",
			"project_id":  projectID,
			"org":         org,
			"repo":        repo,
			"base_branch": baseBranch,
		}))
		runningTasksCount = 0
	}

	topOfQueuePatchID := ""
	topOfQueueStatus := ""
	topOfQueueSHA := ""
	if oldestPatch != nil {
		topOfQueuePatchID = oldestPatch.Id.Hex()
		topOfQueueStatus = oldestPatch.Status
		topOfQueueSHA = oldestPatch.GithubMergeData.HeadSHA
	}

	// Emit span using WithNewRoot to ignore sampling so this always gets exported.
	_, span := tracer.Start(ctx, "merge_queue.depth_sample",
		trace.WithNewRoot(),
		trace.WithAttributes(
			attribute.String("evergreen.merge_queue.project_id", projectID),
			attribute.String("evergreen.merge_queue.org", org),
			attribute.String("evergreen.merge_queue.repo", repo),
			attribute.String("evergreen.merge_queue.queue_name", baseBranch),
			attribute.String("evergreen.merge_queue.base_branch", baseBranch),
			attribute.Int64("evergreen.merge_queue.depth", depth),
			attribute.Int64("evergreen.merge_queue.pending_count", pendingCount),
			attribute.Int64("evergreen.merge_queue.running_count", runningCount),
			attribute.Int64("evergreen.merge_queue.running_tasks_count", int64(runningTasksCount)),
			attribute.Bool("evergreen.merge_queue.has_running_tasks", runningTasksCount > 0),
			attribute.Int64("evergreen.merge_queue.oldest_patch_age_ms", oldestPatchAgeMs),
			attribute.String("evergreen.merge_queue.top_of_queue_patch_id", topOfQueuePatchID),
			attribute.String("evergreen.merge_queue.top_of_queue_status", topOfQueueStatus),
			attribute.String("evergreen.merge_queue.top_of_queue_sha", topOfQueueSHA),
		))
	span.End()

	return nil
}

// PopulateMergeQueueMetricsJobs enqueues a job to emit merge queue depth metrics.
func PopulateMergeQueueMetricsJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.WithStack(err)
		}

		if flags.MonitorDisabled {
			return nil
		}

		j := NewMergeQueueMetricsJob()
		return queue.Put(ctx, j)
	}
}
