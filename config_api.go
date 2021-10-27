package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ClientBinary struct {
	Arch        string `yaml:"arch" json:"arch"`
	OS          string `yaml:"os" json:"os"`
	URL         string `yaml:"url" json:"url"`
	DisplayName string `yaml:"display_name" json:"display_name"`
}

type ClientConfig struct {
	ClientBinaries   []ClientBinary `yaml:"client_binaries" json:"client_binaries"`
	S3ClientBinaries []ClientBinary `yaml:"s3_client_binaries" json:"s3_client_binaries"`
	LatestRevision   string         `yaml:"latest_revision" json:"latest_revision"`
}

// APIConfig holds relevant log and listener settings for the API server.
type APIConfig struct {
	HttpListenAddr      string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	GithubWebhookSecret string `bson:"github_webhook_secret" json:"github_webhook_secret" yaml:"github_webhook_secret"`
}

func (c *APIConfig) SectionId() string { return "api" }

func (c *APIConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = APIConfig{}
			return nil
		}

		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *APIConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"http_listen_addr":      c.HttpListenAddr,
			"github_webhook_secret": c.GithubWebhookSecret,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *APIConfig) ValidateAndDefault() error { return nil }
