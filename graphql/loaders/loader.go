package loaders

import (
	"context"
	"net/http"
	"time"

	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/vikstrous/dataloadgen"
)

type ctxKey string

const loadersKey = ctxKey("dataloaders")

// Loaders contains all dataloader instances for batching GraphQL queries.
type Loaders struct {
	APIVersionLoader *dataloadgen.Loader[string, *restModel.APIVersion]
	UserLoader       *dataloadgen.Loader[string, *restModel.APIDBUser]
}

// New instantiates data loaders for the middleware.
func New() *Loaders {
	ur := &userReader{}
	vr := &apiVersionReader{}
	return &Loaders{
		UserLoader:       dataloadgen.NewMappedLoader(ur.getUsers, dataloadgen.WithWait(time.Millisecond)),
		APIVersionLoader: dataloadgen.NewMappedLoader(vr.getAPIVersions, dataloadgen.WithWait(time.Millisecond)),
	}
}

// Middleware injects data loaders into the request context.
func Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l := New()
		r = r.WithContext(context.WithValue(r.Context(), loadersKey, l))
		next.ServeHTTP(w, r)
	})
}

// For returns the dataloader for a given context.
func For(ctx context.Context) *Loaders {
	return ctx.Value(loadersKey).(*Loaders)
}
