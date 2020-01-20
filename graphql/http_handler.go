package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler() func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New()))

	return srv.ServeHTTP
}
