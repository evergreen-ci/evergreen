package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	mergeQueueRemovalDetailJobName = "merge-queue-removal-detail"

	// mergeQueueRemovalEventMatchWindow is how long before the destroyed webhook a GitHub removal
	// event can be and still be attributed to it. Older events are from a previous time the PR
	// left the queue.
	mergeQueueRemovalEventMatchWindow = time.Minute
)

func init() {
	registry.AddJobType(mergeQueueRemovalDetailJobName, func() amboy.Job { return makeMergeQueueRemovalDetailJob() })
}

type mergeQueueRemovalDetailJob struct {
	job.Base  `bson:"job_base" json:"job_base" yaml:"job_base"`
	ProjectID string    `bson:"project_id" json:"project_id" yaml:"project_id"`
	Org       string    `bson:"org" json:"org" yaml:"org"`
	Repo      string    `bson:"repo" json:"repo" yaml:"repo"`
	HeadRef   string    `bson:"head_ref" json:"head_ref" yaml:"head_ref"`
	HeadSHA   string    `bson:"head_sha" json:"head_sha" yaml:"head_sha"`
	Reason    string    `bson:"reason" json:"reason" yaml:"reason"`
	RemovedAt time.Time `bson:"removed_at" json:"removed_at" yaml:"removed_at"`
}

func makeMergeQueueRemovalDetailJob() *mergeQueueRemovalDetailJob {
	return &mergeQueueRemovalDetailJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    mergeQueueRemovalDetailJobName,
				Version: 0,
			},
		},
	}
}

// NewMergeQueueRemovalDetailJob creates a job that looks up why GitHub removed a merge group's PR
// from the merge queue and records it in a span.
func NewMergeQueueRemovalDetailJob(projectID, org, repo, headRef, headSHA, reason string, removedAt time.Time) amboy.Job {
	j := makeMergeQueueRemovalDetailJob()
	j.ProjectID = projectID
	j.Org = org
	j.Repo = repo
	j.HeadRef = headRef
	j.HeadSHA = headSHA
	j.Reason = reason
	j.RemovedAt = removedAt
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", mergeQueueRemovalDetailJobName, org, repo, headSHA))
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(4),
		WaitUntil:   utility.ToTimeDurationPtr(30 * time.Second),
	})
	return j
}

// Run fetches the PR's latest GitHub merge queue removal event and emits it as a span, retrying
// while GitHub has not recorded it yet.
func (j *mergeQueueRemovalDetailJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	projectRef, err := model.FindBranchProjectRef(ctx, j.ProjectID)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "finding project ref '%s'", j.ProjectID))
		return
	}
	if projectRef == nil {
		j.AddError(errors.Errorf("project ref '%s' not found", j.ProjectID))
		return
	}

	event, err := thirdparty.GetLatestMergeQueueRemovalEvent(ctx, j.Org, j.Repo, j.HeadRef)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting merge queue removal event"))
		return
	}
	j.handleRemovalEvent(ctx, projectRef.Identifier, event)
}

func (j *mergeQueueRemovalDetailJob) handleRemovalEvent(ctx context.Context, projectIdentifier string, event *thirdparty.MergeQueueRemovalEvent) {
	if event != nil && event.CreatedAt.Before(j.RemovedAt.Add(-mergeQueueRemovalEventMatchWindow)) {
		event = nil
	}
	// GitHub may not have recorded the removal event yet when the webhook arrives. If it never
	// does, the PR likely stayed in the queue (e.g. its group was invalidated by an earlier
	// group's failure), so the span still goes out on the last attempt without detail.
	if event == nil && j.RetryInfo().GetRemainingAttempts() > 0 {
		j.AddRetryableError(errors.New("merge queue removal event not found yet"))
		return
	}

	patch.EmitMergeQueueRemovalDetailSpan(ctx, projectIdentifier, j.Org, j.Repo, j.HeadRef, j.HeadSHA, j.Reason, event)
}
