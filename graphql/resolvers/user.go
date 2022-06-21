package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

func (r *userResolver) Permissions(ctx context.Context, obj *restModel.APIDBUser) (*gqlModel.Permissions, error) {
	return &gqlModel.Permissions{UserID: utility.FromStringPtr(obj.UserID)}, nil
}

// User returns generated.UserResolver implementation.
func (r *Resolver) User() generated.UserResolver { return &userResolver{r} }

type userResolver struct{ *Resolver }
