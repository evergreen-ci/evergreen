package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
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
	requesters := patchesInput.Requesters
	if utility.FromBoolPtr(patchesInput.OnlyMergeQueue) {
		requesters = []string{evergreen.GithubMergeRequester}
	}
	opts := patch.ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project:       obj.Id,
		PatchName:     patchesInput.PatchName,
		Statuses:      patchesInput.Statuses,
		Page:          patchesInput.Page,
		Limit:         patchesInput.Limit,
		IncludeHidden: patchesInput.IncludeHidden,
		Requesters:    requesters,
		CountLimit:    utility.FromIntPtr(patchesInput.CountLimit),
	}

	// Check which fields are requested in the query
	selectedFields := graphql.CollectAllFields(ctx)
	queriedPatches := false
	queriedCount := false
	for _, field := range selectedFields {
		if field == "patches" {
			queriedPatches = true
		}
		if field == "filteredPatchCount" {
			queriedCount = true
		}
	}

	// Fetch patches only if requested
	var err error
	apiPatches := []*restModel.APIPatch{}
	if queriedPatches {
		patches, err := patch.ByPatchNameStatusesMergeQueuePaginatedResults(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches for project '%s': %s", utility.FromStringPtr(opts.Project), err.Error()))
		}

		for _, p := range patches {
			apiPatch := restModel.APIPatch{}
			err = apiPatch.BuildFromService(ctx, p, nil) // Injecting DB info into APIPatch is handled by the resolvers.
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch '%s' to APIPatch: %s", p.Id.Hex(), err.Error()))
			}
			apiPatches = append(apiPatches, &apiPatch)
		}
	}

	// Fetch count only if requested
	count := 0
	if queriedCount && len(apiPatches) == patchesInput.Limit {
		count, err = patch.ByPatchNameStatusesMergeQueuePaginatedCount(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch count for project '%s': %s", utility.FromStringPtr(opts.Project), err.Error()))
		}
	}

	return &Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

type projectResolver struct{ *Resolver }
