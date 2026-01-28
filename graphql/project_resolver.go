package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// IsFavorite is the resolver for the isFavorite field.
func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	projectIdentifier := utility.FromStringPtr(obj.Identifier)
	p, err := model.FindBranchProjectRef(ctx, projectIdentifier)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	if p == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectIdentifier))
	}
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *obj.Identifier) {
		return true, nil
	}
	return false, nil
}

// Patches is the resolver for the patches field.
func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput PatchesInput) (*Patches, error) {
	// Return empty Patches - field resolvers will access patchesInput via Parent.Args
	return &Patches{}, nil
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

type projectResolver struct{ *Resolver }
