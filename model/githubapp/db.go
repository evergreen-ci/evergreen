package githubapp

import "github.com/mongodb/anser/bsonutil"

// GitHubAppCollection contains information about Evergreen's GitHub app
// installations for internal use. This does not contain project-owned GitHub
// apps.
const GitHubAppCollection = "github_hooks"

//nolint:megacheck,unused
var (
	ownerKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey  = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	appIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "AppID")
)
