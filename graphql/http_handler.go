package graphql

import (
	"context"
	"errors"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/ravilushqa/otelgqlgen"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/otel/attribute"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string, allowMutations bool) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New(apiURL)))

	// Send OTEL traces for each request.
	// Only create spans for resolved fields.
	srv.Use(otelgqlgen.Middleware(
		otelgqlgen.WithCreateSpanFromFields(func(fieldCtx *graphql.FieldContext) bool {
			return fieldCtx.IsMethod
		}),
		otelgqlgen.WithRequestVariablesAttributesBuilder(
			otelgqlgen.RequestVariablesBuilderFunc(func(requestVariables map[string]any) []attribute.KeyValue {
				redactedRequestVariables := RedactFieldsInMap(requestVariables, redactedFields)
				flattenedVariables := flattenOtelVariables(redactedRequestVariables)

				return otelgqlgen.RequestVariables(flattenedVariables)
			}),
		),
	))

	// Log graphql requests to splunk
	srv.Use(SplunkTracing{})

	// Disable queries for service degradation
	srv.Use(DisableQuery{})

	if !allowMutations {
		srv.Use(DisableMutations{})
	}

	// Handler to log graphql panics to splunk
	srv.SetRecoverFunc(func(ctx context.Context, err any) error {
		queryPath := graphql.GetFieldContext(ctx).Path()

		grip.Critical(message.Fields{
			"path":    "/graphql/query",
			"message": "unhandled panic",
			"error":   err,
			"stack":   string(debug.Stack()),
			"request": gimlet.GetRequestID(ctx),
			"query":   queryPath,
		})
		return errors.New("internal server error")
	})

	srv.SetErrorPresenter(func(ctx context.Context, err error) *gqlerror.Error {
		fieldCtx := graphql.GetFieldContext(ctx)
		queryPath := ""
		args := map[string]any{}
		if fieldCtx != nil {
			queryPath = fieldCtx.Path().String()
			args = fieldCtx.Args
		}
		args = RedactFieldsInMap(args, redactedFields)
		if err != nil && !strings.HasSuffix(err.Error(), context.Canceled.Error()) {
			grip.Error(message.WrapError(err, message.Fields{
				"path":    "/graphql/query",
				"query":   queryPath,
				"args":    args,
				"request": gimlet.GetRequestID(ctx),
			}))
		}
		return graphql.DefaultErrorPresenter(ctx, err)
	})
	return srv.ServeHTTP
}
