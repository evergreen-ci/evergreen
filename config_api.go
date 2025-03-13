package evergreen

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

		checkFailedMsg := message.Fields{
			"message": "could not check for existence of Evergreen client binary for this OS/arch, skipping it and continuing app startup",
			"os":      clientBinary.OS,
			"arch":    clientBinary.Arch,
			"url":     clientBinary.URL,
		}
		// Check that the client exists and is accessible.
		req, err := http.NewRequestWithContext(ctx, http.MethodHead, clientBinary.URL, nil)
		if err != nil {
			grip.Notice(message.WrapError(err, checkFailedMsg))
			continue
		}
		resp, err := client.Do(req)
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
	HttpListenAddr string `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	URL            string `bson:"url" json:"url" yaml:"url"`
}

func (c *APIConfig) SectionId() string { return "api" }

func (c *APIConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

var (
	httpListenAddrKey = bsonutil.MustHaveTag(APIConfig{}, "HttpListenAddr")
	urlKey            = bsonutil.MustHaveTag(APIConfig{}, "URL")
)

func (c *APIConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			httpListenAddrKey: c.HttpListenAddr,
			urlKey:            c.URL,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *APIConfig) ValidateAndDefault() error {
	if c.URL == "" {
		return errors.New("URL must not be empty")
	}
	return nil
}
