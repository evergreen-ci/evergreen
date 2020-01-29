package graphql

import (
	"net/http"

	"github.com/99designs/gqlgen/graphql/handler"
	// "github.com/evergreen-ci/gimlet"
)

// Handler returns a gimlet http handler func used as the gql route handler
func Handler(apiURL string) func(w http.ResponseWriter, r *http.Request) {
	srv := handler.NewDefaultServer(NewExecutableSchema(New(apiURL)))

	// usr := gimlet.GetUser(r.Context())

	return srv.ServeHTTP
}
