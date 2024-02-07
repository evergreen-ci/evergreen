package graphql

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/ravilushqa/otelgqlgen"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"go.opentelemetry.io/otel/attribute"
)

const (
	requestVariablesPrefix = "gql.request.variables"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New(apiURL)))

	// Send OTEL traces for each request.
	// Only create spans for resolved fields.
	srv.Use(otelgqlgen.Middleware(
		otelgqlgen.WithCreateSpanFromFields(func(fieldCtx *graphql.FieldContext) bool {
			return fieldCtx.IsMethod
		}),
		otelgqlgen.WithRequestVariablesAttributesBuilder(
			otelgqlgen.RequestVariablesBuilderFunc(func(requestVariables map[string]interface{}) []attribute.KeyValue {
				// Deep clone the request variables to avoid modifying the original map.
				redactedRequestVariables := map[string]interface{}{}
				registeredTypes := []interface{}{
					map[string]interface{}{},
				}
				err := util.DeepCopy(requestVariables, &redactedRequestVariables, registeredTypes)
				if err != nil {
					grip.Error(message.WrapError(err, message.Fields{
						"message": "failed to deep copy request variables",
					}))
					return nil
				}
				RedactFieldsInMap(redactedRequestVariables)
				variables := make([]attribute.KeyValue, 0, len(redactedRequestVariables))

				for k, v := range redactedRequestVariables {
					variables = append(variables, attribute.String(fmt.Sprintf("%s.%s", requestVariablesPrefix, k), fmt.Sprintf("%+v", v)))

				}
				return variables
			}),
		),
	))

	// Log graphql requests to splunk
	srv.Use(SplunkTracing{})

	// Disable queries for service degradation
	srv.Use(DisableQuery{})

	// Handler to log graphql panics to splunk
	srv.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
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
		args := map[string]interface{}{}
		if fieldCtx != nil {
			queryPath = fieldCtx.Path().String()
			args = fieldCtx.Args
		}
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
