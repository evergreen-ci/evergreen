package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
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
	minimumStuckTime               = 5 * time.Minute
	// pendingIntentMaxAge is how long an unprocessed merge intent is treated as
	// in flight regardless of its processor job. After that, it's only in flight
	// if its processor job is still in the queue and not completed; otherwise
	// the job was lost (e.g. never enqueued or crashed) and the intent would
	// block recovery indefinitely.
	pendingIntentMaxAge = 30 * time.Minute
	// unprocessedIntentLookback bounds how far back to look for unprocessed
	// merge intents. It matches GitHub's maximum merge queue status check
	// timeout, after which the merge group is removed from the queue anyway.
	unprocessedIntentLookback = 6 * time.Hour
	// minQueueAge is how long a patch must have been in the merge queue per the database before the job polls GitHub
	//to verify that its merge group is still queued.
	minQueueAge = 10 * time.Minute
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
// Evergreen patch.
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

	// A merge group whose intent has been received but not yet processed into
	// a patch already has a pending intent. Treat the head SHAs of intents that
	// are still in flight as covered so we never create a second patch for the
	// same merge group.
	now := time.Now()
	unprocessedIntents, err := patch.FindUnprocessedGithubMergeIntents(ctx, now.Add(-unprocessedIntentLookback))
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "finding unprocessed merge queue intents"))
		return
	}
	pending := map[string]bool{}
	for _, intent := range unprocessedIntents {
		if isMergeIntentInFlight(ctx, j.env.RemoteQueue(), intent, now) {
			pending[intent.HeadSHA] = true
		}
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
		if err := j.syncProjectQueue(ctx, &projectRefs[i]); err != nil {
			grip.Error(ctx, message.WrapError(err, message.Fields{
				"message":    "syncing merge queue patches with GitHub queue entries for project",
				"project_id": projectRefs[i].Id,
				"job_id":     j.ID(),
			}))
			j.AddRetryableError(err)
		}
	}
}

// isMergeIntentInFlight returns whether an unprocessed merge intent may still be
// processed into a patch, either because it's recent or because its processor
// job is still waiting or running in the queue.
func isMergeIntentInFlight(ctx context.Context, q amboy.Queue, intent patch.UnprocessedMergeIntent, now time.Time) bool {
	if now.Sub(intent.CreatedAt) < pendingIntentMaxAge {
		return true
	}
	processor, ok := q.Get(ctx, patchIntentProcessorJobID(patch.GithubMergeIntentType, intent.ID))
	return ok && !processor.Status().Completed
}

// recoverProject creates a patch for the merge group at the front of the project's GitHub merge
// queue (queue position 1) if it has neither an active patch nor a pending intent.
func (j *mergeQueuePatchRecoveryJob) recoverProject(ctx context.Context, projectRef *model.ProjectRef, pending map[string]bool) error {
	frontRef, frontSHA, ok, err := thirdparty.GetMergeQueueFrontEntry(ctx, projectRef.Owner, projectRef.Repo, projectRef.Branch)
	if err != nil {
		return errors.Wrap(err, "getting merge queue front entry")
	}
	if !ok || pending[frontSHA] {
		return nil
	}

	// We use the patch intent job's timeout as the grace period because once it has passed, the job that
	// inserted the patch has been cancelled, so an older unfinalized patch is a leftover and shouldn't block a new one.
	existingPatchID, err := patch.FindDuplicateMergeQueuePatchID(ctx, projectRef.Id, projectRef.Owner, projectRef.Repo, frontSHA, "", maxPatchIntentJobTime)
	if err != nil {
		return errors.Wrap(err, "checking for an existing merge queue patch")
	}
	if existingPatchID != "" {
		return nil
	}

	if err := j.recoverMergeGroup(ctx, projectRef, frontRef, frontSHA); err != nil {
		return errors.Wrapf(err, "recovering merge group with head SHA '%s'", frontSHA)
	}
	return nil
}

// recoverMergeGroup creates a patch for a staged merge group that has no
// Evergreen patch.
func (j *mergeQueuePatchRecoveryJob) recoverMergeGroup(ctx context.Context, projectRef *model.ProjectRef, headRef, headSHA string) error {
	headCommit, err := thirdparty.GetCommitEvent(ctx, projectRef.Owner, projectRef.Repo, headSHA)
	if err != nil {
		return errors.Wrap(err, "getting merge group head commit")
	}

	stagedAt := mergeGroupStagedAt(headCommit)
	// Only recover a patch if the queue doesn't have patch since the last recover job.
	if !stagedAt.IsZero() && time.Since(stagedAt) < minimumStuckTime {
		return nil
	}

	baseSHA := firstParentSHA(headCommit)
	if baseSHA == "" {
		return errors.Errorf("merge group head commit '%s' has no parent to use as the base SHA", headSHA)
	}

	event := newMergeGroupEvent(projectRef.Owner, projectRef.Repo, headRef, headSHA, baseSHA, headCommit)
	msgID := fmt.Sprintf("%s-%s-%s", mergeQueuePatchRecoveryJobName, headSHA, mgobson.NewObjectId().Hex())
	intent, err := patch.NewGithubMergeIntent(ctx, msgID, patch.AutomatedCaller, event)
	if err != nil {
		return errors.Wrap(err, "creating merge intent")
	}
	if err := intent.Insert(ctx); err != nil {
		return errors.Wrap(err, "inserting merge intent")
	}
	processor := NewGitHubPatchIntentProcessor(j.env, mgobson.NewObjectId(), intent, projectRef.Id)
	if err := j.env.RemoteQueue().Put(ctx, processor); err != nil {
		return errors.Wrap(err, "enqueueing merge queue patch intent processor")
	}

	grip.Info(ctx, message.Fields{
		"message":    "created missing merge queue patch",
		"project_id": projectRef.Id,
		"head_ref":   headRef,
		"head_sha":   headSHA,
		"job_id":     j.ID(),
	})
	return nil
}

// MergeQueueItem identifies one GitHub merge queue.
type MergeQueueItem struct {
	org        string
	repo       string
	baseBranch string
}

// syncProjectQueue removes merge queue patches that the database still shows as queued but
// that GitHub's merge queue does not show.
func (j *mergeQueuePatchRecoveryJob) syncProjectQueue(ctx context.Context, projectRef *model.ProjectRef) error {
	patches, err := patch.FindMergeQueuePatchesByProject(ctx, projectRef.Id)
	if err != nil {
		return errors.Wrapf(err, "finding merge queue patches for project '%s'", projectRef.Id)
	}
	if len(patches) == 0 {
		return nil
	}

	// Patches missing queue metadata cannot be matched to a GitHub merge queue.
	queuePatches := make(map[MergeQueueItem][]patch.Patch)
	for i := range patches {
		p := patches[i]
		if p.GithubMergeData.Org == "" || p.GithubMergeData.Repo == "" || p.GithubMergeData.BaseBranch == "" {
			continue
		}
		key := MergeQueueItem{
			org:        p.GithubMergeData.Org,
			repo:       p.GithubMergeData.Repo,
			baseBranch: p.GithubMergeData.BaseBranch,
		}
		queuePatches[key] = append(queuePatches[key], p)
	}

	catcher := grip.NewSimpleCatcher()
	for key, queuePatchList := range queuePatches {
		if err := j.reconcileQueue(ctx, projectRef.Id, key, queuePatchList); err != nil {
			catcher.Wrapf(err, "syncing merge queue for org '%s', repo '%s', branch '%s'", key.org, key.repo, key.baseBranch)
		}
	}
	return catcher.Resolve()
}

// reconcileQueue polls GitHub for the entries of one merge queue and updates stale ones.
func (j *mergeQueuePatchRecoveryJob) reconcileQueue(ctx context.Context, projectID string, key MergeQueueItem, queuePatches []patch.Patch) error {
	entries, err := thirdparty.GetMergeQueueEntries(ctx, key.org, key.repo, key.baseBranch)
	if err != nil {
		return errors.Wrapf(err, "getting merge queue entries for org '%s', repo '%s', branch '%s'", key.org, key.repo, key.baseBranch)
	}
	githubEntries := make(map[string]bool, len(entries))
	for _, sha := range entries {
		githubEntries[sha] = true
	}

	catcher := grip.NewSimpleCatcher()
	for _, headSHA := range getStaleQueueItems(queuePatches, githubEntries, time.Now()) {
		updatedPatches, err := patch.MarkMergeQueuePatchesRemovedFromQueue(ctx, key.org, key.repo, headSHA, thirdparty.MergeQueueReasonRecovered)
		if err != nil {
			catcher.Wrapf(err, "marking merge queue patches with head SHA '%s' as removed", headSHA)
			continue
		}

		patchIDs := make([]string, 0, len(updatedPatches))
		for i := range updatedPatches {
			p := &updatedPatches[i]
			patchIDs = append(patchIDs, p.Id.Hex())
			// Patches may not be finalized when the merge group was destroyed before the patch intent was processed into a version with tasks.
			// CancelPatch would delete such patches, so those are only marked removed.
			if p.Version == "" {
				continue
			}
			catcher.Wrapf(model.CancelPatch(ctx, p, task.AbortInfo{User: evergreen.GithubMergeUser}), "cancelling patch '%s'", p.Id.Hex())
		}
	}
	return catcher.Resolve()
}

// getStaleQueueItems returns the distinct head SHAs of Evergreen patches that the database that claims they are
// in the merge queue but is not.
func getStaleQueueItems(patches []patch.Patch, githubEntries map[string]bool, now time.Time) []string {
	var stale []string
	seen := make(map[string]bool, len(patches))
	for i := range patches {
		p := &patches[i]
		headSHA := p.GithubMergeData.HeadSHA
		if headSHA == "" || seen[headSHA] || githubEntries[headSHA] {
			continue
		}
		seen[headSHA] = true

		queueEntryTime := p.GithubMergeData.HeadCommitDate
		if queueEntryTime.IsZero() {
			queueEntryTime = p.CreateTime
		}
		if now.Sub(queueEntryTime) >= minQueueAge {
			stale = append(stale, headSHA)
		}
	}
	return stale
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
