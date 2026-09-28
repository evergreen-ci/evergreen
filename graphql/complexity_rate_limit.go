package graphql

import (
	"context"
	"slices"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/ratelimit"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const errComplexityRateLimit = "COMPLEXITY_RATE_LIMIT_EXCEEDED"

// ComplexityRateLimit is a gqlgen extension that enforces a per-user
// cumulative complexity budget per hour. Each query's static complexity
// score is charged against the user's budget via a Redis token bucket.
type ComplexityRateLimit struct {
	env    evergreen.Environment
	schema graphql.ExecutableSchema
}

var _ interface {
	graphql.OperationContextMutator
	graphql.HandlerExtension
} = &ComplexityRateLimit{}

func NewComplexityRateLimit(env evergreen.Environment, schema graphql.ExecutableSchema) *ComplexityRateLimit {
	return &ComplexityRateLimit{env: env, schema: schema}
}

func (*ComplexityRateLimit) ExtensionName() string {
	return "ComplexityRateLimit"
}

func (c *ComplexityRateLimit) Validate(schema graphql.ExecutableSchema) error {
	c.schema = schema
	return nil
}

func (c *ComplexityRateLimit) MutateOperationContext(ctx context.Context, opCtx *graphql.OperationContext) *gqlerror.Error {
	u := gimlet.GetUser(ctx)
	if u == nil {
		return nil
	}

	op := opCtx.Doc.Operations.ForName(opCtx.OperationName)
	if op == nil {
		return nil
	}
	score := complexity.Calculate(ctx, c.schema, op, opCtx.Variables)
	if score <= 0 {
		return nil
	}

	cfg := c.env.Settings().RateLimit
	perHour := cfg.GraphQLComplexityPerHour
	burst := cfg.GraphQLComplexityBurst
	if perHour == 0 {
		return nil
	}

	username := u.Username()
	elevated := slices.Contains(cfg.ElevatedUserIDs, username)
	if elevated {
		perHour *= 2
		burst *= 2
	}
	exempt := slices.Contains(cfg.ExemptUserIDs, username)

	limiter, err := ratelimit.NewRateLimiter(c.env.RedisClient())
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "initializing complexity rate limiter",
			"user":    username,
		}))
		return nil
	}

	res, err := limiter.AllowN(ctx, username, evergreen.RateLimitSurfaceComplexity, perHour, burst, score)
	if err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message": "checking complexity rate limit",
			"user":    username,
			"score":   score,
		}))
		return nil
	}
	if res == nil {
		return nil
	}

	exceeded := res.Allowed == 0
	if exceeded && !exempt {
		flags, _ := evergreen.GetServiceFlags(ctx)
		if flags != nil && !flags.GraphQLComplexityRateLimiterDisabled {
			isService := false
			if dbUser, ok := u.(*user.DBUser); ok {
				isService = dbUser.OnlyAPI
			}
			grip.Warning(ctx, message.Fields{
				"message":     "complexity rate limit exceeded, rejecting query",
				"user":        username,
				"score":       score,
				"is_service":  isService,
				"remaining":   res.Remaining,
				"retry_after": res.RetryAfter.Seconds(),
			})
			gqlErr := gqlerror.Errorf("complexity rate limit exceeded (score %d)", score)
			errcode.Set(gqlErr, errComplexityRateLimit)
			return gqlErr
		}
	}

	return nil
}
