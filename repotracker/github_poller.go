package repotracker

import (
	"encoding/base64"
	"fmt"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/shelman/angier"
	"time"
)

var (
	_ fmt.Stringer = nil
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
func isLastRevision(revision string,
	githubCommit *thirdparty.GithubCommit) bool {
	if githubCommit.SHA == revision {
		return true
	}
	return false
}

// githubCommitToRevision converts a GithubCommit struct to a
// model.Revision struct
func githubCommitToRevision(
	githubCommit *thirdparty.GithubCommit) model.Revision {
	return model.Revision{
		Author:          githubCommit.Commit.Author.Name,
		AuthorEmail:     githubCommit.Commit.Author.Email,
		RevisionMessage: githubCommit.Commit.Message,
		Revision:        githubCommit.SHA,
		CreateTime:      time.Now(),
	}
}

// GetRemoteConfig fetches the contents of a remote github repository's
// configuration data as at a given revision
func (gRepoPoller *GithubRepositoryPoller) GetRemoteConfig(
	projectFileRevision string) (projectConfig *model.Project, err error) {
	// find the project configuration file for the given repository revision
	projectRef := gRepoPoller.ProjectRef
	projectFileURL := thirdparty.GetGithubFileURL(
		projectRef.Owner,
		projectRef.Repo,
		projectRef.RemotePath,
		projectFileRevision,
	)

	githubFile, err := thirdparty.GetGithubFile(
		gRepoPoller.OauthToken,
		projectFileURL,
	)
	if err != nil {
		return nil, err
	}

	projectFileBytes, err := base64.StdEncoding.DecodeString(githubFile.Content)
	if err != nil {
		return nil, thirdparty.FileDecodeError{err.Error()}
	}

	projectConfig = &model.Project{}
	err = model.LoadProjectInto(projectFileBytes, projectConfig)
	if err != nil {
		return nil, thirdparty.YAMLFormatError{err.Error()}
	}
	return projectConfig, angier.TransferByFieldNames(projectRef, projectConfig)
}

// GetRevisionsSince fetches the all commits from the corresponding Github
// ProjectRef that were made after 'revision'
func (gRepoPoller *GithubRepositoryPoller) GetRevisionsSince(revision string,
	maxRevisionsToSearch int) (revisions []model.Revision, err error) {
	commitURL := getCommitURL(gRepoPoller.ProjectRef)
	var foundLatest bool

	for len(revisions) < maxRevisionsToSearch {
		githubCommits, header, err := thirdparty.GetGithubCommits(
			gRepoPoller.OauthToken, commitURL)
		if err != nil {
			return nil, err
		}

		for _, commit := range githubCommits {
			if isLastRevision(revision, &commit) {
				foundLatest = true
				break
			}
			revisions = append(revisions, githubCommitToRevision(
				&commit))
		}

		// stop querying for commits if we've found the latest commit or got
		// back no commits
		if foundLatest || len(revisions) == 0 {
			break
		}

		// stop quering for commits if there's no next page
		if commitURL = thirdparty.NextGithubPageLink(header); commitURL == "" {
			break
		}
	}

	if !foundLatest {
		return []model.Revision{}, fmt.Errorf("Unable to locate "+
			"requested revision “%v” within “%v” revisions", revision,
			maxRevisionsToSearch)
	}

	return
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
