package route

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/google/go-github/v52/github"
)

type githubComments struct {
	baseUrl string
}

func newGithubComments(baseUrl string) *githubComments {
	return &githubComments{baseUrl: baseUrl}
}

// existingPatches returns a comment that is posted when there are existing patches for a commit SHA and
// they will not be cancelled, so the PR context is of the existing rather than the new PR.
func (g *githubComments) existingPatches(patches []patch.Patch) string {
	return fmt.Sprintf("There is an existing patch(s) for this commit SHA:\n%s\n\nPlease note that the status that is posted is not in the context of this PR but rather the (latest) existing patch and that may affect some tests that may depend on the particular PR. If your tests do not rely on any PR-specific values (like base or head branch name) than your tests will report the same status. If you would like a patch to run in the context of this PR and cancel the other(s), comment 'evergreen retry'.", g.getLinksForPRPatches(patches))
}

// overriddingPR returns a comment that is posted when there are existing patches for a commit SHA and
// they will be cancelled, so the PR context is of the new PR rather than the existing PR.
func (g *githubComments) overriddingPR(patches []patch.Patch) string {
	return fmt.Sprintf("There is an existing patch(s) for this commit SHA that will be cancelled:\n%s\n\nThe status reported will be corresponding to this PR rather than the previous existing ones. If you would like a patch to run for another PR and to cancel this one, comment 'evergreen retry' on the corresponding PR.", g.getLinksForPRPatches(patches))
}

// overriddenPR returns a comment that is posted when a PR's patch is cancelled (or changed) in
// favor of anther patch in a different PR context.
func (g *githubComments) overriddenPR(pr *github.PullRequest) string {
	return fmt.Sprintf("Another [PR](%s) with the same head SHA has ran 'evergreen retry' and overridden this PR's patch. This PR's patch will be canceled and the status reported will be in the context of the other PR.", createGitHubPRLink(pr.Base.User.GetLogin(), pr.Base.Repo.GetName(), pr.GetNumber()))
}

// getLinksForPRPatches returns a string of links for patches
// and if a PR number is available, it will also return a link to the PR.
func (g *githubComments) getLinksForPRPatches(patches []patch.Patch) string {
	links := ""
	for _, p := range patches {
		patchLink := fmt.Sprintf("%s/version/%s", g.baseUrl, p.Id.Hex())
		links += fmt.Sprintf(" - Evergreen [patch](%s)", patchLink)
		if p.GithubPatchData.PRNumber > 0 {
			owner := p.GithubPatchData.BaseOwner
			repo := p.GithubPatchData.BaseRepo
			prNum := p.GithubPatchData.PRNumber
			links += fmt.Sprintf(" with [PR](%s)", createGitHubPRLink(owner, repo, prNum))
		}
		links += "\n"
	}
	return links
}

func createGitHubPRLink(owner, repo string, prNum int) string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, prNum)
}
