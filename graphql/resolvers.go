package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}

type patchResolver struct{ *Resolver }

func (r *patchResolver) ID(ctx context.Context, obj *model.APIPatch) (string, error) {
	return *obj.Id, nil
}

type queryResolver struct{ *Resolver }

func (r *queryResolver) UserPatches(ctx context.Context, userID string) ([]*model.APIPatch, error) {
	patchPointers := []*model.APIPatch{}
	patches, err := r.sc.FindPatchesByUser(userID, time.Now(), 10)
	if err != nil {
		return patchPointers, errors.Wrap(err, "error retrieving patches")
	}

	for _, p := range patches {
		patchPointers = append(patchPointers, &p)
	}

	return patchPointers, nil
}

// New injects resources into the resolvers, such as the data connector
func New() Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{},
		},
	}
}
