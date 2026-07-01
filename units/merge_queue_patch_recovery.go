package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	mergeQueuePatchRecoveryJobName = "merge-queue-patch-recovery"

	// mergeQueueRecoveryMinStagedAge is how long a merge group must have been
	// staged (per its merge commit's creation time) before this job creates a
	// patch for it. It exceeds the 5-minute cron interval so a merge group staged
	// shortly before a run is not recovered while GitHub's merge_group webhook may
	// still be in flight, which would race the webhook and create a duplicate
	// patch. A genuinely stuck merge group is recovered on a later run once it is
	// old enough.
	mergeQueueRecoveryMinStagedAge = 10 * time.Minute
)

func init() {
	registry.AddJobType(mergeQueuePatchRecoveryJobName, NewMergeQueuePatchRecoveryJob)
}

type mergeQueuePatchRecoveryJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewMergeQueuePatchRecoveryJob returns a job that checks every project with the
// GitHub merge queue enabled for merge groups GitHub has staged but that have no
// Evergreen patch — which happens when GitHub fails to deliver the merge_group
// webhook — and creates the missing patch so the merge queue does not hang.
func NewMergeQueuePatchRecoveryJob() amboy.Job {
	j := &mergeQueuePatchRecoveryJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    mergeQueuePatchRecoveryJobName,
				Version: 0,
			},
		},
	}
	j.SetID(fmt.Sprintf("%s.%s", mergeQueuePatchRecoveryJobName, utility.RoundPartOfHour(5).Format(TSFormat)))
	const maxAttempts = 3
	const maxTime = 2 * time.Minute
	// A single global instance avoids two runs creating patches for the same
	// merge group concurrently.
	j.SetScopes([]string{mergeQueuePatchRecoveryJobName})
	j.SetEnqueueAllScopes(true)
	j.SetTimeInfo(amboy.JobTimeInfo{MaxTime: maxTime})
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(maxAttempts),
	})
	return j
}

func (j *mergeQueuePatchRecoveryJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	projectRefs, err := model.FindProjectRefsWithMergeQueueEnabled(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "finding projects with merge queue enabled"))
		return
	}

	// A merge group whose webhook intent has been received but not yet processed
	// into a patch already has a pending intent. Treat those head SHAs as covered
	// so we never create a second patch for the same merge group.
	pendingHeadSHAs, err := patch.FindUnprocessedGithubMergeIntentHeadSHAs(ctx)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "finding unprocessed merge queue intents"))
		return
	}
	pending := make(map[string]bool, len(pendingHeadSHAs))
	for _, sha := range pendingHeadSHAs {
		pending[sha] = true
	}

	for i := range projectRefs {
		if err := j.recoverProject(ctx, &projectRefs[i], pending); err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message":    "recovering missing merge queue patches for project",
				"project_id": projectRefs[i].Id,
				"job_id":     j.ID(),
			}))
			j.AddRetryableError(err)
		}
	}
}

// recoverProject creates a patch for any merge group GitHub has staged for the
// project's merge queue that has neither an active patch nor a pending intent.
func (j *mergeQueuePatchRecoveryJob) recoverProject(ctx context.Context, projectRef *model.ProjectRef, pending map[string]bool) error {
	existingPatches, err := patch.FindMergeQueuePatchesByProject(ctx, projectRef.Id)
	if err != nil {
		return errors.Wrap(err, "finding active merge queue patches")
	}
	covered := make(map[string]bool, len(existingPatches))
	for _, p := range existingPatches {
		covered[p.GithubMergeData.HeadSHA] = true
	}

	refs, err := thirdparty.ListMergeQueueRefs(ctx, projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return errors.Wrap(err, "listing merge queue refs")
	}

	catcher := grip.NewBasicCatcher()
	for _, ref := range refs {
		headSHA := ref.GetObject().GetSHA()
		headRef := ref.GetRef()
		if headSHA == "" || headRef == "" {
			continue
		}
		if covered[headSHA] || pending[headSHA] {
			continue
		}
		created, err := j.recoverMergeGroup(ctx, projectRef, headRef, headSHA)
		if err != nil {
			catcher.Wrapf(err, "recovering merge group with head SHA '%s'", headSHA)
			continue
		}
		if created {
			// Guard against creating a second patch for another ref in this same
			// run that resolves to the same head SHA.
			covered[headSHA] = true
		}
	}
	return catcher.Resolve()
}

// recoverMergeGroup creates a patch for a staged merge group that has no
// Evergreen patch, reusing the same intent → patch pipeline as the webhook
// handler. It returns false without creating anything when the merge group was
// staged too recently to be confident GitHub's webhook was lost rather than
// merely still in flight.
func (j *mergeQueuePatchRecoveryJob) recoverMergeGroup(ctx context.Context, projectRef *model.ProjectRef, headRef, headSHA string) (bool, error) {
	headCommit, err := thirdparty.GetCommitEvent(ctx, projectRef.Owner, projectRef.Repo, headSHA)
	if err != nil {
		return false, errors.Wrap(err, "getting merge group head commit")
	}

	stagedAt := mergeGroupStagedAt(headCommit)
	if stagedAt.IsZero() || time.Since(stagedAt) < mergeQueueRecoveryMinStagedAge {
		grip.Debug(ctx, message.Fields{
			"message":    "merge group staged too recently; deferring recovery to avoid racing the webhook",
			"project_id": projectRef.Id,
			"head_ref":   headRef,
			"head_sha":   headSHA,
			"staged_at":  stagedAt,
			"job_id":     j.ID(),
		})
		return false, nil
	}

	baseSHA := firstParentSHA(headCommit)
	if baseSHA == "" {
		return false, errors.Errorf("merge group head commit '%s' has no parent to use as the base SHA", headSHA)
	}

	event := newMergeGroupEvent(projectRef.Owner, projectRef.Repo, headRef, headSHA, baseSHA, headCommit)
	msgID := fmt.Sprintf("%s-%s-%s", mergeQueuePatchRecoveryJobName, headSHA, mgobson.NewObjectId().Hex())
	intent, err := patch.NewGithubMergeIntent(ctx, msgID, patch.AutomatedCaller, event)
	if err != nil {
		return false, errors.Wrap(err, "creating merge intent")
	}
	if err := intent.Insert(ctx); err != nil {
		return false, errors.Wrap(err, "inserting merge intent")
	}
	processor := NewPatchIntentProcessor(j.env, mgobson.NewObjectId(), intent)
	if err := j.env.RemoteQueue().Put(ctx, processor); err != nil {
		return false, errors.Wrap(err, "enqueueing merge queue patch intent processor")
	}

	grip.Info(ctx, message.Fields{
		"message":    "created missing merge queue patch",
		"project_id": projectRef.Id,
		"head_ref":   headRef,
		"head_sha":   headSHA,
		"job_id":     j.ID(),
	})
	return true, nil
}

// mergeGroupStagedAt returns when GitHub created the merge group's commit, which
// approximates when the merge group entered the queue. It prefers the committer
// date (set by GitHub when it forms the group) and falls back to the author date.
func mergeGroupStagedAt(commit *github.RepositoryCommit) time.Time {
	if commit == nil || commit.Commit == nil {
		return time.Time{}
	}
	if committer := commit.Commit.GetCommitter(); committer != nil && !committer.GetDate().Time.IsZero() {
		return committer.GetDate().Time
	}
	return commit.Commit.GetAuthor().GetDate().Time
}

// firstParentSHA returns the SHA of the merge group commit's first parent, which
// is the base commit the merge group was built on.
func firstParentSHA(commit *github.RepositoryCommit) string {
	if commit == nil || len(commit.Parents) == 0 {
		return ""
	}
	return commit.Parents[0].GetSHA()
}

// newMergeGroupEvent builds the GitHub merge_group event that the patch-creation
// path expects, from data reconstructed via the GitHub API rather than a webhook.
func newMergeGroupEvent(owner, repo, headRef, headSHA, baseSHA string, headCommit *github.RepositoryCommit) *github.MergeGroupEvent {
	mergeGroup := &github.MergeGroup{
		HeadRef: github.Ptr(headRef),
		HeadSHA: github.Ptr(headSHA),
		BaseSHA: github.Ptr(baseSHA),
	}
	if headCommit != nil {
		mergeGroup.HeadCommit = headCommit.Commit
	}
	return &github.MergeGroupEvent{
		Org:        &github.Organization{Login: github.Ptr(owner)},
		Repo:       &github.Repository{Name: github.Ptr(repo)},
		MergeGroup: mergeGroup,
	}
}

// PopulateMergeQueuePatchRecoveryJobs enqueues the merge queue patch recovery
// job. It is gated behind a service flag so it can be turned off without a deploy.
func PopulateMergeQueuePatchRecoveryJobs() amboy.QueueOperation {
	return func(ctx context.Context, queue amboy.Queue) error {
		flags, err := evergreen.GetServiceFlags(ctx)
		if err != nil {
			return errors.Wrap(err, "getting service flags")
		}
		if !flags.MergeQueueRecoveryEnabled {
			return nil
		}
		return queue.Put(ctx, NewMergeQueuePatchRecoveryJob())
	}
}
