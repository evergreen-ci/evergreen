package graphql

import (
	"context"
	"net/http"
	"time"

	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/vikstrous/dataloadgen"
)

type ctxKey string

const loadersKey = ctxKey("dataloaders")

type Loaders struct {
	UserLoader    *dataloadgen.Loader[string, *restModel.APIDBUser]
	VersionLoader *dataloadgen.Loader[string, *restModel.APIVersion]
	TaskLoader    *dataloadgen.Loader[string, *restModel.APITask]
}

// NewLoaders instantiates data loaders for the middleware.
func NewLoaders() *Loaders {
	ur := &userReader{}
	vr := &versionReader{}
	tr := &taskReader{}
	return &Loaders{
		UserLoader:    dataloadgen.NewLoader(ur.getUsers, dataloadgen.WithWait(time.Millisecond)),
		VersionLoader: dataloadgen.NewLoader(vr.getVersions, dataloadgen.WithWait(time.Millisecond)),
		TaskLoader:    dataloadgen.NewLoader(tr.getTasks, dataloadgen.WithWait(time.Millisecond)),
	}
}

// Middleware injects data loaders into the request context.
func DataloaderMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loader := NewLoaders()
		r = r.WithContext(context.WithValue(r.Context(), loadersKey, loader))
		next.ServeHTTP(w, r)
	})
}

// For returns the dataloader for a given context.
func DataloaderFor(ctx context.Context) *Loaders {
	return ctx.Value(loadersKey).(*Loaders)
}
