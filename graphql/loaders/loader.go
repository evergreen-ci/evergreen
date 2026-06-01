package loaders

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/vikstrous/dataloadgen"
)

type ctxKey string

const loadersKey = ctxKey("dataloaders")

// Loaders contains all dataloader instances for batching GraphQL queries.
type Loaders struct {
	UserLoader    *dataloadgen.Loader[string, *user.DBUser]
	VersionLoader *dataloadgen.Loader[string, *model.Version]
}

// loaderWait is how long each dataloader waits for additional keys before
// firing its batch. dataloadgen sets a default of 16ms which is a bit high.
const loaderWait = 5 * time.Millisecond

// New instantiates data loaders for the middleware.
func New() *Loaders {
	ur := &userReader{}
	vr := &versionReader{}
	return &Loaders{
		UserLoader:    dataloadgen.NewMappedLoader(ur.getUsers, dataloadgen.WithWait(loaderWait)),
		VersionLoader: dataloadgen.NewMappedLoader(vr.getVersions, dataloadgen.WithWait(loaderWait)),
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
