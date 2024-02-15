package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// SplunkTracing is a graphql extension that adds splunk logging to graphql.
// It is used to log the duration of a query and the user that made the request.
// It does this by hooking into lifecycle events that gqlgen uses.
type SplunkTracing struct{}

func (SplunkTracing) ExtensionName() string {
	return "SplunkTracing"
}

func (SplunkTracing) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (SplunkTracing) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
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

	defer func() {
		usr := gimlet.GetUser(ctx)
		end := graphql.Now()

		duration := end.Sub(start)
		redactedRequestVariables := RedactFieldsInMap(rc.Variables, redactedFields)
		grip.Info(message.Fields{
			"message":     "graphql.tracing",
			"query":       rc.Operation.Name,
			"operation":   rc.Operation.Operation,
			"variables":   redactedRequestVariables,
			"duration_ms": duration.Milliseconds(),
			"request":     gimlet.GetRequestID(ctx),
			"start":       start,
			"end":         end,
			"user":        usr.Username(),
			"origin":      rc.Headers.Get("Origin"),
		})

	}()
	return next(ctx)
}
