package repotracker

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GithubRepositoryPoller is a struct that implements GitHub specific behavior
// required of a RepoPoller
type GithubRepositoryPoller struct {
	ProjectRef *model.ProjectRef
}

// NewGithubRepositoryPoller constructs and returns a pointer to a
// GithubRepositoryPoller struct
func NewGithubRepositoryPoller(projectRef *model.ProjectRef) *GithubRepositoryPoller {
	return &GithubRepositoryPoller{
		ProjectRef: projectRef,
	}
}

// isLastRevision compares a GitHub commit's SHA with a revision and returns
// true if they are the same.
func isLastRevision(revision string, repoCommit *github.RepositoryCommit) bool {
	if repoCommit.SHA == nil {
		return false
	}
	return *repoCommit.SHA == revision
}

// githubCommitToRevision converts a GitHub repository commit to Evergreen's
// revision model.
func githubCommitToRevision(repoCommit *github.RepositoryCommit) model.Revision {
	r := model.Revision{
		Author:          repoCommit.Commit.Author.GetName(),
		AuthorEmail:     repoCommit.Commit.Author.GetEmail(),
		RevisionMessage: repoCommit.Commit.GetMessage(),
		Revision:        repoCommit.GetSHA(),
		CreateTime:      repoCommit.Commit.Committer.GetDate().Time,
	}

	if repoCommit.Author != nil && repoCommit.Author.ID != nil {
		r.AuthorGithubUID = int(*repoCommit.Author.ID)
	}

	return r
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetRemoteConfig(ctx context.Context, projectFileRevision string) (model.ProjectInfo, error) {
	// find the project configuration file for the given repository revision
	projectRef := gRepoPoller.ProjectRef
	opts := model.GetProjectOpts{
		Ref:        projectRef,
		RemotePath: projectRef.RemotePath,
		Revision:   projectFileRevision,
	}
	return model.GetProjectFromFile(ctx, opts)
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetChangedFiles(ctx context.Context, commitRevision string) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// get the entire commit, then pull the files from it
	projectRef := gRepoPoller.ProjectRef
	commit, err := thirdparty.GetCommitEvent(ctx,
		projectRef.Owner,
		projectRef.Repo,
		commitRevision,
	)
	if err != nil {
		return nil, errors.Wrapf(err, "error loading commit '%v'", commitRevision)
	}

	files := []string{}
	for _, f := range commit.Files {
		if f.Filename == nil {
			return nil, errors.New("received invalid data from github: nil filname")
		}
		files = append(files, *f.Filename)
	}
	return files, nil
}

// GetRevisionsSince fetches the all commits from the corresponding project ref that were made after
// 'revision'. If it finds the revision within the maxRevisionsToSearch limit, it will return all
// commits more recent than that revision, in order of most recent to least recent. Otherwise, if it
// cannot find the revision, it will attempt to add the base revision between the most recent commit
// and the given revision.
func (gRepoPoller *GithubRepositoryPoller) GetRevisionsSince(revision string, maxRevisionsToSearch int) ([]model.Revision, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	var foundLatest bool
	var commits []*github.RepositoryCommit
	var firstCommit *github.RepositoryCommit // we track this for later error handling
	var commitPage int
	revisions := []model.Revision{}

	for len(revisions) < maxRevisionsToSearch {
		var err error
		commits, commitPage, err = thirdparty.GetGithubCommits(ctx, gRepoPoller.ProjectRef.Owner,
			gRepoPoller.ProjectRef.Repo, gRepoPoller.ProjectRef.Branch, time.Time{}, commitPage)
		if err != nil {
			return nil, err
		}

		// Note that commits within a commit page are ordered from most to least recent commit, and
		// commits pages are also ordered from most to least recent, so the resulting revisions are
		// in most to least recent order.
		for i := range commits {
			if len(revisions) >= maxRevisionsToSearch {
				break
			}

			commit := commits[i]
			if commit == nil {
				return nil, errors.Errorf("GitHub commit history returned a nil commit for project ref '%s'", gRepoPoller.ProjectRef.Id)
			}
			if firstCommit == nil {
				firstCommit = commit
			}

			if commit.Commit == nil || commit.Commit.Author == nil ||
				commit.Commit.Author.Name == nil ||
				commit.Commit.Author.Email == nil ||
				commit.Commit.Message == nil ||
				commit.SHA == nil ||
				commit.Commit.Committer == nil ||
				commit.Commit.Committer.Date == nil {
				return nil, errors.Errorf("GitHub returned commit history with missing information for project ref '%s'", gRepoPoller.ProjectRef.Id)
			}

			if isLastRevision(revision, commit) {
				foundLatest = true
				break
			}
			revisions = append(revisions, githubCommitToRevision(commit))
		}

		// stop querying for commits if we've found the latest commit or got back no commits
		if foundLatest || commitPage == 0 {
			break
		}
	}

	if !foundLatest {
		var err error
		var baseRevision string

		// Attempt to get the merge base commit between the given revision (i.e. what Evergreen
		// thinks is the most recent revision) and the most recent commit (i.e. the branch's actual
		// most recent commit).
		if firstCommit != nil {
			githubCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			baseRevision, err = thirdparty.GetGithubMergeBaseRevision(
				githubCtx,
				gRepoPoller.ProjectRef.Owner,
				gRepoPoller.ProjectRef.Repo,
				revision,
				*firstCommit.SHA,
			)
		} else {
			err = errors.New("no recent commit found")
		}
		if len(revision) < 10 {
			return nil, errors.Errorf("invalid revision '%s'", revision)
		}
		if err != nil {
			// unable to get merge base commit so set projectRef revision details with a blank base revision
			revisionDetails := &model.RepositoryErrorDetails{
				Exists:            true,
				InvalidRevision:   revision[:10],
				MergeBaseRevision: "",
			}
			revisionError := errors.Wrapf(err,
				"unable to find a suggested merge base commit for revision '%s', must fix on projects settings page",
				revision)
			if err := gRepoPoller.ProjectRef.SetRepotrackerError(revisionDetails); err != nil {
				return []model.Revision{}, errors.Wrap(err, "setting repotracker error")
			}
			return []model.Revision{}, revisionError
		}

		// automatically set the newly found base revision as base revision and append revisions
		commit, err := thirdparty.GetCommitEvent(ctx,
			gRepoPoller.ProjectRef.Owner,
			gRepoPoller.ProjectRef.Repo,
			baseRevision,
		)
		if err != nil {
			return nil, errors.Wrapf(err, "loading base commit '%s'", baseRevision)
		}
		revisions = append(revisions, githubCommitToRevision(commit))

		grip.Info(message.Fields{
			"message":            "updating last repo revision for project",
			"source":             "github poller",
			"old_revision":       revision,
			"new_revision":       baseRevision,
			"project":            gRepoPoller.ProjectRef.Id,
			"project_identifier": gRepoPoller.ProjectRef.Identifier,
		})
		if err = model.UpdateLastRevision(gRepoPoller.ProjectRef.Id, baseRevision); err != nil {
			return nil, errors.Wrapf(err, "updating last revision to base revision '%s'", baseRevision)
		}
	}

	if len(revisions) == 0 {
		commitSHAs := make([]string, 0, len(commits))
		for i := range commits {
			commitSHAs = append(commitSHAs, commits[i].GetSHA())
		}
		grip.Info(message.Fields{
			"source":             "github poller",
			"message":            "no new revisions",
			"last_revision":      revision,
			"project":            gRepoPoller.ProjectRef.Id,
			"project_identifier": gRepoPoller.ProjectRef.Identifier,
			"commits":            commitSHAs,
		})
	}

	return revisions, nil
}

// GetRecentRevisions fetches the most recent 'numRevisions'
func (gRepoPoller *GithubRepositoryPoller) GetRecentRevisions(maxRevisions int) ([]model.Revision, error) {
	ctx, cancel := context.WithTimeout(context.TODO(), 30*time.Second)
	defer cancel()

	var revisions []model.Revision
	commitPage := 0

	for {
		var err error
		var repoCommits []*github.RepositoryCommit
		repoCommits, commitPage, err = thirdparty.GetGithubCommits(ctx, gRepoPoller.ProjectRef.Owner,
			gRepoPoller.ProjectRef.Repo, gRepoPoller.ProjectRef.Branch,
			time.Time{}, commitPage)
		if err != nil {
			return nil, err
		}

		for _, commit := range repoCommits {
			if len(revisions) == maxRevisions {
				break
			}
			revisions = append(revisions, githubCommitToRevision(
				commit))
		}

		// stop querying for commits if we've reached our target
		if len(revisions) == maxRevisions || commitPage == 0 {
			break
		}
	}

	return revisions, nil
}
