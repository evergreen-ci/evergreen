package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/apollotracing"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New(apiURL)))

	// Apollo tracing support https://github.com/apollographql/apollo-tracing
	srv.Use(extension.AutomaticPersistedQuery{Cache: lru.New(100)})
	srv.Use(apollotracing.Tracer{})

	return srv.ServeHTTP
}
