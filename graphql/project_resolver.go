package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	p, err := model.FindBranchProjectRef(*obj.Identifier)
	if err != nil || p == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.Identifier, err))
	}
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *obj.Identifier) {
		return true, nil
	}
	return false, nil
}

func (r *projectResolver) ValidDefaultLoggers(ctx context.Context, obj *restModel.APIProjectRef) ([]string, error) {
	return model.ValidDefaultLoggers, nil
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

type projectResolver struct{ *Resolver }
