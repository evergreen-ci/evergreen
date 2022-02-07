package graphql

import (
	"context"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type (
	SplunkTracing struct{}
)

var _ interface {
	graphql.HandlerExtension
	graphql.ResponseInterceptor
} = SplunkTracing{}

func (SplunkTracing) ExtensionName() string {
	return "SplunkTracing"
}

func (SplunkTracing) Validate(graphql.ExecutableSchema) error {
	return nil
}

func (SplunkTracing) InterceptResponse(ctx context.Context, next graphql.ResponseHandler) *graphql.Response {
	rc := graphql.GetOperationContext(ctx)

	start := graphql.Now()

	defer func() {
		end := graphql.Now()

		duration := end.Sub(start)
		grip.Info(message.Fields{
			"message":     "graphql tracing",
			"ui_query":    rc.Operation.Name,
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
