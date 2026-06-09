package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

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

func (c *RateLimitConfig) SectionId() string { return "rate_limit" }

func (c *RateLimitConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
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

func (c *RateLimitConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			rateLimitRESTHumanPerHourKey:      c.RESTHumanRequestsPerHour,
			rateLimitRESTHumanBurstKey:        c.RESTHumanBurst,
			rateLimitRESTServicePerHourKey:    c.RESTServiceRequestsPerHour,
			rateLimitRESTServiceBurstKey:      c.RESTServiceBurst,
			rateLimitGraphQLHumanPerHourKey:   c.GraphQLHumanRequestsPerHour,
			rateLimitGraphQLHumanBurstKey:     c.GraphQLHumanBurst,
			rateLimitGraphQLServicePerHourKey: c.GraphQLServiceRequestsPerHour,
			rateLimitGraphQLServiceBurstKey:   c.GraphQLServiceBurst,
			rateLimitComplexityLimitKey:       c.GraphQLComplexityLimit,
			rateLimitElevatedUserIDsKey:       c.ElevatedUserIDs,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *RateLimitConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	validateRateLimitPair(c.RESTHumanRequestsPerHour, c.RESTHumanBurst, "REST human", catcher)
	validateRateLimitPair(c.RESTServiceRequestsPerHour, c.RESTServiceBurst, "REST service", catcher)
	validateRateLimitPair(c.GraphQLHumanRequestsPerHour, c.GraphQLHumanBurst, "GraphQL human", catcher)
	validateRateLimitPair(c.GraphQLServiceRequestsPerHour, c.GraphQLServiceBurst, "GraphQL service", catcher)
	if c.GraphQLComplexityLimit < 0 {
		catcher.New("GraphQL complexity limit must be non-negative")
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
