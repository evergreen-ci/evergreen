package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type OwnerRepo struct {
	Owner string `bson:"owner" json:"owner" yaml:"owner"`
	Repo  string `bson:"repo" json:"repo" yaml:"repo"`
}

type ProjectCreationConfig struct {
	// TotalProjectLimit is the total number of projects that Evergreen is willing to support
	TotalProjectLimit int `bson:"total_project_limit" json:"total_project_limit" yaml:"total_project_limit"`

	// RepoProjectLimit is the number of projects that Evergreen will allow each repo to have
	RepoProjectLimit int `bson:"repo_project_limit" json:"repo_project_limit" yaml:"repo_project_limit"`

	// RepoExceptions is a list of repos that can override the default repo-project limit but not the total project limit
	RepoExceptions []OwnerRepo `bson:"repo_exceptions,omitempty" json:"repo_exceptions" yaml:"repo_exceptions"`
}

var (
	ProjectCreationConfigTotalProjectLimitKey = bsonutil.MustHaveTag(ProjectCreationConfig{}, "TotalProjectLimit")
	ProjectCreationConfigRepoProjectLimitKey  = bsonutil.MustHaveTag(ProjectCreationConfig{}, "RepoProjectLimit")
	ProjectCreationConfigRepoExceptionsKey    = bsonutil.MustHaveTag(ProjectCreationConfig{}, "RepoExceptions")
)

func (*ProjectCreationConfig) SectionId() string { return "project_creation" }

func (c *ProjectCreationConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *ProjectCreationConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{"$set": c}), "updating config section '%s'", c.SectionId())
}

func (c *ProjectCreationConfig) ValidateAndDefault() error { return nil }

// IsExceptionToRepoLimit returns bool if the given owner repo combination has been specified as an exception in admin settings
func (c *ProjectCreationConfig) IsExceptionToRepoLimit(owner, repo string) bool {
	for _, exception := range c.RepoExceptions {
		if exception.Owner == owner && exception.Repo == repo {
			return true
		}
	}
	return false
}
