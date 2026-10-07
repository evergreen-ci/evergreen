package graphql

import (
	"context"
	"slices"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/ratelimit"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// testResultsFetchComplexity is the complexity cost of downloading a task's
// full set of test results.
const testResultsFetchComplexity = 5
const errComplexityRateLimit = "COMPLEXITY_RATE_LIMIT_EXCEEDED"

// ComplexityRateLimit is a gqlgen extension that enforces a per-user
// cumulative complexity budget per hour.
type ComplexityRateLimit struct {
	env     evergreen.Environment
	limiter *ratelimit.Limiter
	schema  graphql.ExecutableSchema
}

var _ interface {
	graphql.OperationContextMutator
	graphql.HandlerExtension
} = &ComplexityRateLimit{}

// NewComplexityRateLimit returns a complexity rate limiter backed by the
// environment's Redis client.
func NewComplexityRateLimit(env evergreen.Environment) (*ComplexityRateLimit, error) {
	limiter, err := ratelimit.NewRateLimiter(env.RedisClient())
	if err != nil {
		return nil, errors.Wrap(err, "creating rate limiter")
	}
	return &ComplexityRateLimit{env: env, limiter: limiter}, nil
}

func (*ComplexityRateLimit) ExtensionName() string {
	return "ComplexityRateLimit"
}

func (c *ComplexityRateLimit) Validate(schema graphql.ExecutableSchema) error {
	c.schema = schema
	return nil
}

func (c *ComplexityRateLimit) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	score := c.complexityScore(ctx, opCtx)
	if score <= 0 {
		return nil
	}

	cfg := c.env.Settings().RateLimit
	perHour := cfg.GraphQLComplexityPerHour
	burst := cfg.GraphQLComplexityBurst
	if perHour == 0 {
		return nil
	}

	username := mustHaveUser(ctx).Username()
	if slices.Contains(cfg.ElevatedUserIDs, username) {
		perHour *= 2
		burst *= 2
	}

	res, err := c.limiter.AllowN(ctx, username, evergreen.RateLimitSurfaceComplexity, perHour, burst, score)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "checking complexity rate limit",
			"user":    username,
			"score":   score,
		}))
		return nil
	}
	if res == nil || res.Allowed > 0 {
		return nil
	}
	// Exempt users still consume from their bucket so their usage is tracked,
	// but they are never rejected.
	if slices.Contains(cfg.ExemptUserIDs, username) {
		return nil
	}

	grip.Warning(ctx, message.Fields{
		"message":     "complexity rate limit exceeded, rejecting query",
		"user":        username,
		"score":       score,
		"remaining":   res.Remaining,
		"retry_after": res.RetryAfter.Seconds(),
	})
	gqlErr := gqlerror.Errorf("complexity rate limit exceeded (score %d)", score)
	errcode.Set(gqlErr, errComplexityRateLimit)
	return gqlErr
}

// complexityScore reuses the score already computed by the per-query
// complexity limiter, and only computes it itself when that limiter is off.
func (c *ComplexityRateLimit) complexityScore(ctx context.Context, opCtx *graphql.OperationContext) int {
	if stats := extension.GetComplexityStats(ctx); stats != nil {
		return stats.Complexity
	}
	return complexity.Calculate(ctx, c.schema, opCtx.Operation, opCtx.Variables)
}

// setComplexityFuncs registers custom complexity calculations for fields
// whose cost is not reflected by gqlgen's default of one point per field.
func setComplexityFuncs(c *Config) {
	c.Complexity.Task.Tests = func(childComplexity int, opts *TestFilterOptions) int {
		if opts == nil {
			return testResultsFetchComplexity + childComplexity
		}
		// Non-nil filter options mean the base task test results are also getting downloaded.
		return 2*testResultsFetchComplexity + childComplexity
	}
}