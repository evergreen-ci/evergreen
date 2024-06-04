package evergreen

import (
	"context"
	"fmt"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	ClientBinaries []ClientBinary
	LatestRevision string
	S3URLPrefix    string
}

func (c *ClientConfig) populateClientBinaries(ctx context.Context, s3URLPrefix string) {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	// We assume that all valid OS/arch combinations listed in Evergreen
	// constants are supported platforms.
	for platform, displayName := range ValidArchDisplayNames {
		osAndArch := strings.Split(platform, "_")
		if len(osAndArch) != 2 {
			continue
		}
		binary := "evergreen"
		if strings.Contains(platform, "windows") {
			binary += ".exe"
		}

		clientBinary := ClientBinary{
			// The items in S3 are expected to be of the form <s3_prefix>/<os>_<arch>/evergreen{.exe}
			URL:         fmt.Sprintf("%s/%s/%s", s3URLPrefix, platform, binary),
			OS:          osAndArch[0],
			Arch:        osAndArch[1],
			DisplayName: displayName,
		}

		// Check that the client exists and is accessible.
		resp, err := client.Head(clientBinary.URL)
		checkFailedMsg := message.Fields{
			"message": "could not check for existence of Evergreen client binary for this OS/arch, skipping it and continuing app startup",
			"os":      clientBinary.OS,
			"arch":    clientBinary.Arch,
			"url":     clientBinary.URL,
		}
		if err != nil {
			grip.Notice(message.WrapError(err, checkFailedMsg))
			continue
		}

		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			checkFailedMsg["status_code"] = resp.StatusCode
			grip.Notice(checkFailedMsg)
			continue
		}

		c.ClientBinaries = append(c.ClientBinaries, clientBinary)
	}

	grip.AlertWhen(len(c.ClientBinaries) == 0, message.Fields{
		"message":       "could not find any valid Evergreen client binaries during app startup, the API server will not be able to distribute Evergreen clients",
		"s3_url_prefix": s3URLPrefix,
	})
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
