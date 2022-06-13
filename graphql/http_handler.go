package graphql

import (
	"context"
	"errors"
	"net/http"
	"runtime/debug"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	generated "github.com/evergreen-ci/evergreen/graphql/generated"
	"github.com/evergreen-ci/evergreen/graphql/resolvers"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(generated.NewExecutableSchema(resolvers.New(apiURL)))
	// Apollo tracing support https://github.com/apollographql/apollo-tracing
	srv.Use(apollotracing.Tracer{})

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
		grip.Error(message.WrapError(err, message.Fields{
			"path":    "/graphql/query",
			"query":   queryPath,
			"args":    args,
			"request": gimlet.GetRequestID(ctx),
		}))
		return graphql.DefaultErrorPresenter(ctx, err)
	})
	return srv.ServeHTTP
}
