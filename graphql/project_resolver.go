package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/model"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// IsFavorite is the resolver for the isFavorite field.
func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, utility.FromStringPtr(obj.Identifier)) {
		return true, nil
	}
	return false, nil
}

// Patches is the resolver for the patches field.
func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput PatchesInput) (*Patches, error) {
	// Return empty Patches - field resolvers will access patchesInput via Parent.Args
	return &Patches{}, nil
}

// IsFavorite is the resolver for the isFavorite field.
func (r *projectLiteResolver) IsFavorite(ctx context.Context, obj *model.ProjectRef) (bool, error) {
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, obj.Identifier) {
		return true, nil
	}
	return false, nil
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

// ProjectLite returns ProjectLiteResolver implementation.
func (r *Resolver) ProjectLite() ProjectLiteResolver { return &projectLiteResolver{r} }

type projectResolver struct{ *Resolver }
type projectLiteResolver struct{ *Resolver }
