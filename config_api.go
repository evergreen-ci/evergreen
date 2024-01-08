package evergreen

import (
	"context"
	"fmt"
	"strings"

	"github.com/evergreen-ci/pail"
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
	ClientBinaries   []ClientBinary
	S3ClientBinaries []ClientBinary
	LatestRevision   string
	S3URLPrefix      string
}

func (c *ClientConfig) populateClientBinaries(ctx context.Context, bucket pail.Bucket, prefix string) error {
	if !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	iter, err := bucket.List(ctx, prefix)
	if err != nil {
		return errors.Wrap(err, "listing client bucket")
	}
	for iter.Next(ctx) {
		item := strings.TrimPrefix(iter.Item().Name(), prefix)
		name := strings.Split(item, "/")
		if len(name) != 2 || !strings.Contains(name[1], "evergreen") {
			continue
		}
		displayName, ok := ValidArchDisplayNames[name[0]]
		if !ok {
			continue
		}
		osArchParts := strings.Split(name[0], "_")
		c.ClientBinaries = append(c.ClientBinaries, ClientBinary{
			URL:         fmt.Sprintf("%s/%s", c.S3URLPrefix, item),
			OS:          osArchParts[0],
			Arch:        osArchParts[1],
			DisplayName: displayName,
		})
	}
	if err = iter.Err(); err != nil {
		return errors.Wrap(err, "iterating client bucket contents")
	}

	return nil
}

// APIConfig holds relevant log and listener settings for the API server.
type APIConfig struct {
	HttpListenAddr      string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	GithubWebhookSecret string `bson:"github_webhook_secret" json:"github_webhook_secret" yaml:"github_webhook_secret"`
}

func (c *APIConfig) SectionId() string { return "api" }

func (c *APIConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = APIConfig{}
			return nil
		}

		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *APIConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"http_listen_addr":      c.HttpListenAddr,
			"github_webhook_secret": c.GithubWebhookSecret,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *APIConfig) ValidateAndDefault() error { return nil }
