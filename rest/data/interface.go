package data

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/google/go-github/v34/github"
)

// Connector is an interface that contains all of the methods which
// connect to the service layer of evergreen. These methods abstract the link
// between the service and the API layers, allowing for changes in the
// service architecture without forcing changes to the API.
type Connector interface {
	// Get and Set URL provide access to the main url string of the API.
	GetURL() string
	SetURL(string)
	GetProjectFromFile(context.Context, model.ProjectRef, string, string) (model.ProjectInfo, error)
	CreateVersionFromConfig(context.Context, *model.ProjectInfo, model.VersionMetadata) (*model.Version, error)
	GetGitHubPR(context.Context, string, string, int) (*github.PullRequest, error)
	AddPatchForPr(ctx context.Context, projectRef model.ProjectRef, prNum int, modules []restModel.APIModule, messageOverride string) (*patch.Patch, error)
	IsAuthorizedToPatchAndMerge(context.Context, *evergreen.Settings, UserRepoInfo) (bool, error)
}
