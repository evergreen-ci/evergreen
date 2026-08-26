package graphql

import (
	"context"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// SplunkTracing is a graphql extension that adds splunk logging to graphql.
// It is used to log the duration of a query and the user that made the request.
// It does this by hooking into lifecycle events that gqlgen uses.
type SplunkTracing struct {
	schema graphql.ExecutableSchema
}

// MakeSplunkTracing is a constructor for SplunkTracing that takes in the graphql schema as an argument.
// The schema is used to calculate the complexity score of a query, which is then logged.
func MakeSplunkTracing(schema graphql.ExecutableSchema) SplunkTracing {
	return SplunkTracing{schema: schema}
}

func (SplunkTracing) ExtensionName() string {
	return "SplunkTracing"
}

func (SplunkTracing) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (s SplunkTracing) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	if hasOperationContext := graphql.HasOperationContext(ctx); !hasOperationContext {
		// There was an invalid operation context, so we can't do anything. This could be because the user made a
		// malformed request to GraphQL.
		return next(ctx)
	}

	rc := graphql.GetOperationContext(ctx)

	if rc.Operation == nil {
		// There was an invalid operation this is likely the result of a bad query
		return next(ctx)
	}
	start := graphql.Now()

	aiAgent := rc.Headers.Get(evergreen.GraphQLAIAgentHeader)
	if aiAgent != "" {
		trace.SpanFromContext(ctx).SetAttributes(attribute.String(evergreen.GraphQLAIAgentOtelAttribute, aiAgent))
	}

	complexityScore := complexity.Calculate(ctx, s.schema, rc.Operation, rc.Variables)
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int("gql.request.complexity_score", complexityScore))

	defer func() {
		usr := gimlet.GetUser(ctx)
		end := graphql.Now()

		duration := end.Sub(start)
		redactedRequestVariables := RedactFieldsInMap(ctx, rc.Variables, redactedFields)
		fields := message.Fields{
			"message":          "graphql.tracing",
			"query":            rc.Operation.Name,
			"operation":        rc.Operation.Operation,
			"variables":        redactedRequestVariables,
			"duration_ms":      duration.Milliseconds(),
			"complexity_score": complexityScore,
			"request":          gimlet.GetRequestID(ctx),
			"start":            start,
			"end":              end,
			"user":             usr.Username(),
			"origin":           rc.Headers.Get("Origin"),
		}
		if aiAgent != "" {
			fields["ai_agent"] = aiAgent
		}
		grip.Info(ctx, fields)

	}()
	return next(ctx)
}
