package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type SplunkTracing struct{}

func (SplunkTracing) ExtensionName() string {
	return "SplunkTracing"
}

func (SplunkTracing) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (SplunkTracing) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	rc := graphql.GetOperationContext(ctx)
	if rc == nil {
		// There was an invalid operation context, so we can't do anything
		grip.Critical(message.Fields{
			"message": "no operation context found",
		})
		return next(ctx)
	}

	if rc.Operation == nil {
		// There was an invalid operation this is likely the result of a bad query
		return next(ctx)
	}
	start := graphql.Now()

	defer func() {
		end := graphql.Now()

		duration := end.Sub(start)
		grip.Info(message.Fields{
			"message":     "graphql.tracing",
			"query":       rc.Operation.Name,
			"operation":   rc.Operation.Operation,
			"variables":   rc.Variables,
			"duration_ms": duration.Milliseconds(),
			"request":     gimlet.GetRequestID(ctx),
			"start":       start,
			"end":         end,
		})

	}()
	return next(ctx)
}
