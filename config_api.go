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
	ClientBinaries          []ClientBinary
	LatestRevision          string
	S3URLPrefix             string
	OldestAllowedCLIVersion string

	// These settings are to support a seemless migration for
	// the switch to OAuth. They can be removed in DEVPROD-17405.
	// The corresponding fields inside the APIClientConfig struct
	// should also be removed then.
	OAuthIssuer      string
	OAuthClientID    string
	OAuthConnectorID string
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
			grip.Notice(ctx, message.WrapError(err, checkFailedMsg))
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			grip.Notice(ctx, message.WrapError(err, checkFailedMsg))
			continue
		}

		_ = resp.Body.Close()
		if resp.StatusCode >= 400 {
			checkFailedMsg["status_code"] = resp.StatusCode
			grip.Notice(ctx, checkFailedMsg)
			continue
		}

		c.ClientBinaries = append(c.ClientBinaries, clientBinary)
	}

	grip.AlertWhen(ctx, len(c.ClientBinaries) == 0, message.Fields{
		"message":       "could not find any valid Evergreen client binaries during app startup, the API server will not be able to distribute Evergreen clients",
		"s3_url_prefix": s3URLPrefix,
	})
}

// RateLimitConfig holds per-bucket rate-limit settings that operators can tune at runtime.
// Zero values for numeric fields disable the corresponding limit.
type RateLimitConfig struct {
	RESTHumanRequestsPerHour   int `bson:"rest_human_per_hour" json:"rest_human_per_hour" yaml:"rest_human_per_hour"`
	RESTHumanBurst             int `bson:"rest_human_burst" json:"rest_human_burst" yaml:"rest_human_burst"`
	RESTServiceRequestsPerHour int `bson:"rest_service_per_hour" json:"rest_service_per_hour" yaml:"rest_service_per_hour"`
	RESTServiceBurst           int `bson:"rest_service_burst" json:"rest_service_burst" yaml:"rest_service_burst"`

	GraphQLHumanRequestsPerHour   int `bson:"graphql_human_per_hour" json:"graphql_human_per_hour" yaml:"graphql_human_per_hour"`
	GraphQLHumanBurst             int `bson:"graphql_human_burst" json:"graphql_human_burst" yaml:"graphql_human_burst"`
	GraphQLServiceRequestsPerHour int `bson:"graphql_service_per_hour" json:"graphql_service_per_hour" yaml:"graphql_service_per_hour"`
	GraphQLServiceBurst           int `bson:"graphql_service_burst" json:"graphql_service_burst" yaml:"graphql_service_burst"`

	GraphQLComplexityLimit int      `bson:"graphql_complexity_limit" json:"graphql_complexity_limit" yaml:"graphql_complexity_limit"`
	ElevatedUserIDs        []string `bson:"elevated_user_ids" json:"elevated_user_ids" yaml:"elevated_user_ids"`
}

func (c *RateLimitConfig) validate() error {
	catcher := grip.NewBasicCatcher()
	validateRateLimitPair(c.RESTHumanRequestsPerHour, c.RESTHumanBurst, "REST human", catcher)
	validateRateLimitPair(c.RESTServiceRequestsPerHour, c.RESTServiceBurst, "REST service", catcher)
	validateRateLimitPair(c.GraphQLHumanRequestsPerHour, c.GraphQLHumanBurst, "GraphQL human", catcher)
	validateRateLimitPair(c.GraphQLServiceRequestsPerHour, c.GraphQLServiceBurst, "GraphQL service", catcher)
	if c.GraphQLComplexityLimit < 0 {
		catcher.New("GraphQL complexity limit must be non-negative")
	}
	for _, id := range c.ElevatedUserIDs {
		if id == "" {
			catcher.New("elevated user IDs must not contain empty strings")
			break
		}
	}
	return catcher.Resolve()
}

func validateRateLimitPair(perHour, burst int, label string, catcher grip.Catcher) {
	if perHour < 0 {
		catcher.Errorf("%s requests per hour must be non-negative", label)
	}
	if burst < 0 {
		catcher.Errorf("%s burst must be non-negative", label)
	}
	if perHour > 0 && burst > perHour {
		catcher.Errorf("%s burst (%d) must not exceed requests per hour (%d)", label, burst, perHour)
	}
}

var (
	rateLimitRESTHumanPerHourKey      = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTHumanRequestsPerHour")
	rateLimitRESTHumanBurstKey        = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTHumanBurst")
	rateLimitRESTServicePerHourKey    = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTServiceRequestsPerHour")
	rateLimitRESTServiceBurstKey      = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTServiceBurst")
	rateLimitGraphQLHumanPerHourKey   = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLHumanRequestsPerHour")
	rateLimitGraphQLHumanBurstKey     = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLHumanBurst")
	rateLimitGraphQLServicePerHourKey = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLServiceRequestsPerHour")
	rateLimitGraphQLServiceBurstKey   = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLServiceBurst")
	rateLimitComplexityLimitKey       = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLComplexityLimit")
	rateLimitElevatedUserIDsKey       = bsonutil.MustHaveTag(RateLimitConfig{}, "ElevatedUserIDs")
)

// APIConfig holds relevant log and listener settings for the API server.
type APIConfig struct {
	HttpListenAddr string          `bson:"http_listen_addr" json:"http_listen_addr" yaml:"httplistenaddr"`
	URL            string          `bson:"url" json:"url" yaml:"url"`
	CorpURL        string          `bson:"corp_url" json:"corp_url" yaml:"corp_url"`
	RateLimit      RateLimitConfig `bson:"rate_limit" json:"rate_limit" yaml:"rate_limit"`
}

func (c *APIConfig) SectionId() string { return "api" }

func (c *APIConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

var (
	httpListenAddrKey = bsonutil.MustHaveTag(APIConfig{}, "HttpListenAddr")
	urlKey            = bsonutil.MustHaveTag(APIConfig{}, "URL")
	corpURLKey        = bsonutil.MustHaveTag(APIConfig{}, "CorpURL")
	rateLimitKey      = bsonutil.MustHaveTag(APIConfig{}, "RateLimit")
)

func (c *APIConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			httpListenAddrKey: c.HttpListenAddr,
			urlKey:            c.URL,
			corpURLKey:        c.CorpURL,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitRESTHumanPerHourKey):      c.RateLimit.RESTHumanRequestsPerHour,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitRESTHumanBurstKey):        c.RateLimit.RESTHumanBurst,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitRESTServicePerHourKey):    c.RateLimit.RESTServiceRequestsPerHour,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitRESTServiceBurstKey):      c.RateLimit.RESTServiceBurst,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitGraphQLHumanPerHourKey):   c.RateLimit.GraphQLHumanRequestsPerHour,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitGraphQLHumanBurstKey):     c.RateLimit.GraphQLHumanBurst,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitGraphQLServicePerHourKey): c.RateLimit.GraphQLServiceRequestsPerHour,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitGraphQLServiceBurstKey):   c.RateLimit.GraphQLServiceBurst,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitComplexityLimitKey):       c.RateLimit.GraphQLComplexityLimit,
			bsonutil.GetDottedKeyName(rateLimitKey, rateLimitElevatedUserIDsKey):       c.RateLimit.ElevatedUserIDs,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *APIConfig) ValidateAndDefault() error {
	if c.URL == "" {
		return errors.New("URL must not be empty")
	}
	return c.RateLimit.validate()
}
