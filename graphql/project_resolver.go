package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/parsley"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Commits is the resolver for the commits field.
func (r *projectResolver) Commits(ctx context.Context, obj *restModel.APIProjectRef, options ProjectCommitsOptions) (*ProjectCommits, error) {
	projectID := utility.FromStringPtr(obj.Identifier)
	if err := checkProjectAccess(ctx, projectID, ProjectPermissionTasks, AccessLevelView); err != nil {
		return nil, err
	}

	limit := utility.FromIntPtr(options.Limit)
	if limit > model.MaxWaterfallVersionLimit {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("limit exceeds max limit of %d", model.MaxWaterfallVersionLimit))
	}

	maxOrderOpt := utility.FromIntPtr(options.MaxOrder)

	opts := model.WaterfallOptions{
		Limit:      limit,
		MaxOrder:   maxOrderOpt,
		Requesters: []string{evergreen.RepotrackerVersionRequester},
	}

	activeVersions, err := model.GetActiveWaterfallVersions(ctx, projectID, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting active waterfall versions: %s", err.Error()))
	}

	versionPtrs := make([]*model.Version, 0, len(activeVersions))
	for i := range activeVersions {
		versionPtrs = append(versionPtrs, &activeVersions[i])
	}

	if len(activeVersions) == 0 {
		return &ProjectCommits{
			Versions: versionPtrs,
			Pagination: &ProjectCommitsPagination{
				HasNextPage: false,
			},
		}, nil
	}

	nextPageOrder := activeVersions[len(activeVersions)-1].RevisionOrderNumber
	return &ProjectCommits{
		Versions: versionPtrs,
		Pagination: &ProjectCommitsPagination{
			NextPageOrder: nextPageOrder,
			HasNextPage:   nextPageOrder > 1,
		},
	}, nil
}

// IsFavorite is the resolver for the isFavorite field.
func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, utility.FromStringPtr(obj.Identifier)) {
		return true, nil
	}
	return false, nil
}

// ParsleyFilters is the resolver for the parsleyFilters field.
func (r *projectResolver) ParsleyFilters(ctx context.Context, obj *restModel.APIProjectRef) ([]*parsley.Filter, error) {
	return apiParsleyFiltersToService(obj.ParsleyFilters), nil
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
