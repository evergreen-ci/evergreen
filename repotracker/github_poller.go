package repotracker

import (
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
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
func NewGithubRepositoryPoller(projectRef *model.ProjectRef,
	oauthToken string) *GithubRepositoryPoller {
	return &GithubRepositoryPoller{
		ProjectRef: projectRef,
		OauthToken: oauthToken,
	}
}

// GetCommitURL constructs the required URL to query for Github commits
func getCommitURL(projectRef *model.ProjectRef) string {
	return fmt.Sprintf("https://api.github.com/repos/%v/%v/commits?sha=%v",
		projectRef.Owner,
		projectRef.Repo,
		projectRef.Branch,
	)
}

// isLastRevision compares a Github Commit's sha with a revision and returns
// true if they are the same
func isLastRevision(revision string, githubCommit *thirdparty.GithubCommit) bool {
	return githubCommit.SHA == revision
}

// githubCommitToRevision converts a GithubCommit struct to a
// model.Revision struct
func githubCommitToRevision(githubCommit *thirdparty.GithubCommit) model.Revision {
	return model.Revision{
		Author:          githubCommit.Commit.Author.Name,
		AuthorEmail:     githubCommit.Commit.Author.Email,
		RevisionMessage: githubCommit.Commit.Message,
		Revision:        githubCommit.SHA,
		CreateTime:      util.RoundPartOfMinute(0),
	}
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetRemoteConfig(projectFileRevision string) (projectConfig *model.Project, err error) {
	// find the project configuration file for the given repository revision
	projectRef := gRepoPoller.ProjectRef
	projectFileURL := thirdparty.GetGithubFileURL(
		projectRef.Owner,
		projectRef.Repo,
		projectRef.RemotePath,
		projectFileRevision,
	)

	githubFile, err := thirdparty.GetGithubFile(gRepoPoller.OauthToken, projectFileURL)
	if err != nil {
		return nil, err
	}

	projectFileBytes, err := base64.StdEncoding.DecodeString(githubFile.Content)
	if err != nil {
		return nil, thirdparty.FileDecodeError{err.Error()}
	}

	projectConfig = &model.Project{}
	err = model.LoadProjectInto(projectFileBytes, projectRef.Identifier, projectConfig)
	if err != nil {
		return nil, thirdparty.YAMLFormatError{err.Error()}
	}

	return projectConfig, nil
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetChangedFiles(commitRevision string) ([]string, error) {
	// get the entire commit, then pull the files from it
	projectRef := gRepoPoller.ProjectRef
	commit, err := thirdparty.GetCommitEvent(
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
		files = append(files, f.FileName)
	}
	return files, nil
}

// GetRevisionsSince fetches the all commits from the corresponding Github
// ProjectRef that were made after 'revision'
func (gRepoPoller *GithubRepositoryPoller) GetRevisionsSince(
	revision string, maxRevisionsToSearch int) ([]model.Revision, error) {

	var foundLatest bool
	var commits []thirdparty.GithubCommit
	var firstCommit *thirdparty.GithubCommit // we track this for later error handling
	var header http.Header
	commitURL := getCommitURL(gRepoPoller.ProjectRef)
	revisions := []model.Revision{}

	for len(revisions) < maxRevisionsToSearch {
		var err error
		commits, header, err = thirdparty.GetGithubCommits(gRepoPoller.OauthToken, commitURL)
		if err != nil {
			return nil, err
		}

		for i := range commits {
			commit := &commits[i]
			if firstCommit == nil {
				firstCommit = commit
			}
			if isLastRevision(revision, commit) {
				foundLatest = true
				break
			}
			revisions = append(revisions, githubCommitToRevision(commit))
		}

		// stop querying for commits if we've found the latest commit or got back no commits
		if foundLatest || len(revisions) == 0 {
			break
		}

		// stop quering for commits if there's no next page
		if commitURL = thirdparty.NextGithubPageLink(header); commitURL == "" {
			break
		}
	}

	if !foundLatest {
		var revisionDetails *model.RepositoryErrorDetails
		var revisionError error
		// attempt to get the merge base commit
		baseRevision, err := thirdparty.GetGitHubMergeBaseRevision(
			gRepoPoller.OauthToken,
			gRepoPoller.ProjectRef.Owner,
			gRepoPoller.ProjectRef.Repo,
			revision,
			firstCommit,
		)
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
		} else {
			// update project ref to have an inconsistent status
			revisionDetails = &model.RepositoryErrorDetails{
				Exists:            true,
				InvalidRevision:   revision[:10],
				MergeBaseRevision: baseRevision,
			}
			revisionError = errors.Errorf("base revision, %v not found, suggested base revision, %v found, must confirm on project settings page",
				revision, baseRevision)
		}

		gRepoPoller.ProjectRef.RepotrackerError = revisionDetails
		if err = gRepoPoller.ProjectRef.Upsert(); err != nil {
			return []model.Revision{}, errors.Wrap(err, "unable to update projectRef revision details")
		}

		return []model.Revision{}, revisionError
	}

	return revisions, nil
}

// GetRecentRevisions fetches the most recent 'numRevisions'
func (gRepoPoller *GithubRepositoryPoller) GetRecentRevisions(maxRevisions int) (
	revisions []model.Revision, err error) {
	commitURL := getCommitURL(gRepoPoller.ProjectRef)

	for {
		githubCommits, header, err := thirdparty.GetGithubCommits(
			gRepoPoller.OauthToken, commitURL)
		if err != nil {
			return nil, err
		}

		for _, commit := range githubCommits {
			if len(revisions) == maxRevisions {
				break
			}
			revisions = append(revisions, githubCommitToRevision(
				&commit))
		}

		// stop querying for commits if we've reached our target or got back no
		// commits
		if len(revisions) == maxRevisions || len(revisions) == 0 {
			break
		}

		// stop quering for commits if there's no next page
		if commitURL = thirdparty.NextGithubPageLink(header); commitURL == "" {
			break
		}
	}
	return
}
