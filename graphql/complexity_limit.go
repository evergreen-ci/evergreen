package graphql

import (
	"context"
	"fmt"
	"strconv"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// ComplexityLimit computes the "complexity" of the query given the
// schema. Based on configured settings, it will either block or warn on
// queries that exceed the complexity limit, setting approriate response headers
// via the HTTP response writer stashed in the request context.
type ComplexityLimit struct {
	schema graphql.ExecutableSchema
}

func MakeComplexityLimit(schema graphql.ExecutableSchema) ComplexityLimit {
	return ComplexityLimit{schema: schema}
}

func (ComplexityLimit) ExtensionName() string {
	return "ComplexityLimit"
}

func (ComplexityLimit) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (c ComplexityLimit) MutateOperationContext(ctx context.Context, rc *graphql.OperationContext) *gqlerror.Error {
	score := complexity.Calculate(ctx, c.schema, rc.Operation, rc.Variables)

	// Look up the admin config to decide whether to reject the query or warn only.
	settings, err := evergreen.GetConfigWithoutSecrets(ctx)
	if err != nil {
		// Fail open: if the config can't be read, don't block the query.
		grip.ErrorWhen(ctx, !errors.Is(context.Canceled, err), errors.Wrap(err, "getting Evergreen admin settings"))
		setComplexityResponseHeaders(ctx, score, false)
		return nil
	}

	limit := settings.RateLimit.GraphQLComplexityLimit
	// A non-positive limit means complexity limiting is not configured.
	exceeded := limit > 0 && score > limit
	setComplexityResponseHeaders(ctx, score, exceeded)
	if !exceeded {
		return nil
	}

	// If limiter is disabled, warn with details, but allow the query to proceed.
	if settings.ServiceFlags.GraphQLComplexityLimiterDisabled {
		grip.Warning(ctx, message.Fields{
			"message":          "graphql query exceeds complexity limit, but limiter is disabled",
			"operation":        rc.Operation.Name,
			"complexity_score": score,
			"complexity_limit": limit,
			"request":          gimlet.GetRequestID(ctx),
		})
		return nil
	}

	return ComplexityLimitExceeded.Send(ctx, fmt.Sprintf("operation complexity %d exceeds the limit of %d", score, limit))
}

// Set the informational headers for complexity in the HTTP response
func setComplexityResponseHeaders(ctx context.Context, score int, exceeded bool) {
	// Retrieve HTTP response writer from context
	w, ok := responseWriterFromContext(ctx)
	if !ok {
		return
	}
	w.Header().Set(evergreen.GraphQLComplexityHeader, strconv.Itoa(score))
	if exceeded {
		w.Header().Set(evergreen.GraphQLComplexityExceededHeader, "true")
	}
}
