package repotracker

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// GithubRepositoryPoller is a struct that implements Github specific behavior
// required of a RepoPoller
type GithubRepositoryPoller struct {
	ProjectRef *model.ProjectRef
	OauthToken string
}

// NewGithubRepositoryPoller constructs and returns a pointer to a
//GithubRepositoryPoller struct
func NewGithubRepositoryPoller(projectRef *model.ProjectRef, oauthToken string) *GithubRepositoryPoller {
	return &GithubRepositoryPoller{
		ProjectRef: projectRef,
		OauthToken: oauthToken,
	}
}

// isLastRevision compares a Github Commit's sha with a revision and returns
// true if they are the same
func isLastRevision(revision string, repoCommit *github.RepositoryCommit) bool {
	if repoCommit.SHA == nil {
		return false
	}
	return *repoCommit.SHA == revision
}

// githubCommitToRevision converts a GithubCommit struct to a
// model.Revision struct
func githubCommitToRevision(repoCommit *github.RepositoryCommit) model.Revision {
	r := model.Revision{
		Author:          *repoCommit.Commit.Author.Name,
		AuthorEmail:     *repoCommit.Commit.Author.Email,
		RevisionMessage: *repoCommit.Commit.Message,
		Revision:        *repoCommit.SHA,
		CreateTime:      *repoCommit.Commit.Committer.Date,
	}

	if repoCommit.Author != nil && repoCommit.Author.ID != nil {
		r.AuthorGithubUID = int(*repoCommit.Author.ID)
	}

	return r
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetRemoteConfig(ctx context.Context, projectFileRevision string) (*model.Project, *model.ParserProject, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// find the project configuration file for the given repository revision
	projectRef := gRepoPoller.ProjectRef
	opts := model.GetProjectOpts{
		Ref:        projectRef,
		RemotePath: projectRef.RemotePath,
		Revision:   projectFileRevision,
		Token:      gRepoPoller.OauthToken,
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
		gRepoPoller.OauthToken,
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

// GetRevisionsSince fetches the all commits from the corresponding Github
// ProjectRef that were made after 'revision'
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
		commits, commitPage, err = thirdparty.GetGithubCommits(ctx,
			gRepoPoller.OauthToken, gRepoPoller.ProjectRef.Owner,
			gRepoPoller.ProjectRef.Repo, gRepoPoller.ProjectRef.Branch, time.Time{}, commitPage)
		if err != nil {
			return nil, err
		}

		for i := range commits {
			commit := commits[i]
			if commit == nil {
				return nil, errors.Errorf("github returned commit history with missing information for project ref: %s", gRepoPoller.ProjectRef.Id)
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
				return nil, errors.Errorf("github returned commit history with missing information for project ref: %s", gRepoPoller.ProjectRef.Id)
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
		var revisionDetails *model.RepositoryErrorDetails
		var revisionError error
		var err error
		var baseRevision string

		// attempt to get the merge base commit
		if firstCommit != nil {
			baseRevision, err = thirdparty.GetGithubMergeBaseRevision(
				ctx,
				gRepoPoller.OauthToken,
				gRepoPoller.ProjectRef.Owner,
				gRepoPoller.ProjectRef.Repo,
				revision,
				*firstCommit.SHA,
			)

		} else {
			err = errors.New("no recent commit found")
		}
		if len(revision) < 10 {
			return nil, errors.Errorf("invalid revision: %v", revision)
		}
		if err != nil {
			// unable to get merge base commit so set projectRef revision details with a blank base revision
			revisionDetails = &model.RepositoryErrorDetails{
				Exists:            true,
				InvalidRevision:   revision[:10],
				MergeBaseRevision: "",
			}
			revisionError = errors.Wrapf(err,
				"unable to find a suggested merge base commit for revision %v, must fix on projects settings page",
				revision)
			gRepoPoller.ProjectRef.RepotrackerError = revisionDetails
			if err = gRepoPoller.ProjectRef.Upsert(); err != nil {
				return []model.Revision{}, errors.Wrap(err, "unable to update projectRef revision details")
			}
			return []model.Revision{}, revisionError
		} else {
			// automatically set the newly found base revision as base revision and append revisions
			commit, err := thirdparty.GetCommitEvent(ctx,
				gRepoPoller.OauthToken,
				gRepoPoller.ProjectRef.Owner,
				gRepoPoller.ProjectRef.Repo,
				baseRevision,
			)
			if err != nil {
				return nil, errors.Wrapf(err, "error loading commit '%v'", baseRevision)
			}
			revisions = append(revisions, githubCommitToRevision(commit))

			grip.Info(message.Fields{
				"source":             "github poller",
				"message":            "updating new base revision",
				"old_revision":       revision,
				"new_revision":       baseRevision,
				"project":            gRepoPoller.ProjectRef.Id,
				"project_identifier": gRepoPoller.ProjectRef.Identifier,
			})
			err = model.UpdateLastRevision(gRepoPoller.ProjectRef.Id, baseRevision)
			if err != nil {
				return nil, errors.Wrap(err, "error updating last revision")
			}
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
		repoCommits, commitPage, err = thirdparty.GetGithubCommits(ctx,
			gRepoPoller.OauthToken, gRepoPoller.ProjectRef.Owner,
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
