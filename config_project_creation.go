package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type OwnerRepo struct {
	Owner string `bson:"owner" json:"owner" yaml:"owner"`
	Repo  string `bson:"repo" json:"repo" yaml:"repo"`
}

type ProjectCreationConfig struct {

	// Total number of projects that evergreen is willing to support
	TotalProjectLimit int `bson:"total_project_limit" json:"total_project_limit" yaml:"total_project_limit"`

	// Number of projects that evergreen will allow each repo to have
	RepoProjectLimit int `bson:"repo_project_limit" json:"repo_project_limit" yaml:"repo_project_limit"`

	// List of repos that can override the default repo-project limit but not the total project limit
	ReposToOverride []OwnerRepo `bson:"repos_to_override" json:"repos_to_override" yaml:"repos_to_override"`
}

var (
	ProjectCreationConfigTotalProjectLimitKey = bsonutil.MustHaveTag(ProjectCreationConfig{}, "TotalProjectLimit")
	ProjectCreationConfigRepoProjectLimitKey  = bsonutil.MustHaveTag(ProjectCreationConfig{}, "RepoProjectLimit")
	ProjectCreationConfigReposToOverrideKey   = bsonutil.MustHaveTag(ProjectCreationConfig{}, "ReposToOverride")
)

func (*ProjectCreationConfig) SectionId() string { return "project_creation" }

func (c *ProjectCreationConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = ProjectCreationConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *ProjectCreationConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)
	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{"$set": c}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *ProjectCreationConfig) ValidateAndDefault() error { return nil }
