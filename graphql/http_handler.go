package graphql

import (
	"context"
	"errors"
	"net/http"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New(apiURL)))

	// Apollo tracing support https://github.com/apollographql/apollo-tracing
	srv.Use(apollotracing.Tracer{})

	// Handler to log graphql panics to splunk
	srv.SetRecoverFunc(func(ctx context.Context, err interface{}) error {
		queryPath := graphql.GetFieldContext(ctx).Path()
		grip.Critical(message.Fields{
			"path":    "/graphql/query",
			"message": "unhandled panic",
			"error":   err,
			"query":   queryPath,
		})
		return errors.New("internal server error")
	})
	return srv.ServeHTTP
}
