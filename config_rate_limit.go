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
	RESTUserPerHour    int `bson:"rest_user_per_hour" json:"rest_user_per_hour" yaml:"rest_user_per_hour"`
	RESTUserBurst      int `bson:"rest_user_burst" json:"rest_user_burst" yaml:"rest_user_burst"`
	RESTServicePerHour int `bson:"rest_service_per_hour" json:"rest_service_per_hour" yaml:"rest_service_per_hour"`
	RESTServiceBurst   int `bson:"rest_service_burst" json:"rest_service_burst" yaml:"rest_service_burst"`

	GraphQLUserPerHour    int `bson:"graphql_user_per_hour" json:"graphql_user_per_hour" yaml:"graphql_user_per_hour"`
	GraphQLUserBurst      int `bson:"graphql_user_burst" json:"graphql_user_burst" yaml:"graphql_user_burst"`
	GraphQLServicePerHour int `bson:"graphql_service_per_hour" json:"graphql_service_per_hour" yaml:"graphql_service_per_hour"`
	GraphQLServiceBurst   int `bson:"graphql_service_burst" json:"graphql_service_burst" yaml:"graphql_service_burst"`

	GraphQLComplexityLimit int      `bson:"graphql_complexity_limit" json:"graphql_complexity_limit" yaml:"graphql_complexity_limit"`
	ElevatedUserIDs        []string `bson:"elevated_user_ids" json:"elevated_user_ids" yaml:"elevated_user_ids"`
}

func (c *RateLimitConfig) SectionId() string { return "rate_limit" }

func (c *RateLimitConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

var (
	rateLimitRESTUserPerHourKey       = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTUserPerHour")
	rateLimitRESTUserBurstKey         = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTUserBurst")
	rateLimitRESTServicePerHourKey    = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTServicePerHour")
	rateLimitRESTServiceBurstKey      = bsonutil.MustHaveTag(RateLimitConfig{}, "RESTServiceBurst")
	rateLimitGraphQLUserPerHourKey    = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLUserPerHour")
	rateLimitGraphQLUserBurstKey      = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLUserBurst")
	rateLimitGraphQLServicePerHourKey = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLServicePerHour")
	rateLimitGraphQLServiceBurstKey   = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLServiceBurst")
	rateLimitComplexityLimitKey       = bsonutil.MustHaveTag(RateLimitConfig{}, "GraphQLComplexityLimit")
	rateLimitElevatedUserIDsKey       = bsonutil.MustHaveTag(RateLimitConfig{}, "ElevatedUserIDs")
)

func (c *RateLimitConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			rateLimitRESTUserPerHourKey:       c.RESTUserPerHour,
			rateLimitRESTUserBurstKey:         c.RESTUserBurst,
			rateLimitRESTServicePerHourKey:    c.RESTServicePerHour,
			rateLimitRESTServiceBurstKey:      c.RESTServiceBurst,
			rateLimitGraphQLUserPerHourKey:    c.GraphQLUserPerHour,
			rateLimitGraphQLUserBurstKey:      c.GraphQLUserBurst,
			rateLimitGraphQLServicePerHourKey: c.GraphQLServicePerHour,
			rateLimitGraphQLServiceBurstKey:   c.GraphQLServiceBurst,
			rateLimitComplexityLimitKey:       c.GraphQLComplexityLimit,
			rateLimitElevatedUserIDsKey:       c.ElevatedUserIDs,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *RateLimitConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	validateRateLimitPair(c.RESTUserPerHour, c.RESTUserBurst, "REST user", catcher)
	validateRateLimitPair(c.RESTServicePerHour, c.RESTServiceBurst, "REST service", catcher)
	validateRateLimitPair(c.GraphQLUserPerHour, c.GraphQLUserBurst, "GraphQL user", catcher)
	validateRateLimitPair(c.GraphQLServicePerHour, c.GraphQLServiceBurst, "GraphQL service", catcher)
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
