package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/repos/{repo_id}

type repoIDPatchHandler struct {
	repoID          string
	user            *user.DBUser
	newRepoRef      *dbModel.RepoRef
	originalRepoRef *dbModel.RepoRef
	apiNewRepoRef   *model.APIProjectRef

	settings *evergreen.Settings
}

func makePatchRepoByID(settings *evergreen.Settings) gimlet.RouteHandler {
	return &repoIDPatchHandler{
		settings: settings,
	}
}

// Factory creates an instance of the handler.
//
//	@Summary		Modify a repo
//	@Description	Modify existing repo settings (restricted to repo admins). For lists, if there is a complementary "delete" field, then the former field indicates items to be added, while the "delete" field indicates items to be deleted. Otherwise, the given list will overwrite the original list (the only exception is for project variables -- we will ignore any empty project variables to avoid accidentally overwriting private variables).
//	@Tags			repos
//	@Router			/repos/{repo_id} [patch]
//	@Security		Api-User || Api-Key
//	@Param			repo_id		path		string				true	"the repo ID"
//	@Param			{object}	body		model.APIProjectRef	true	"parameters"
//	@Success		200			{object}	model.APIProjectRef
func (h *repoIDPatchHandler) Factory() gimlet.RouteHandler {
	return &repoIDPatchHandler{
		settings: h.settings,
	}
}

func (h *repoIDPatchHandler) Parse(ctx context.Context, r *http.Request) error {
	h.repoID = gimlet.GetVars(r)["repo_id"]
	h.user = MustHaveUser(ctx)

	body := utility.NewRequestReader(r)
	defer body.Close()
	b, err := io.ReadAll(body)
	if err != nil {
		return errors.Wrap(err, "reading JSON request body")
	}

	oldRepoRef, err := dbModel.FindOneRepoRef(ctx, h.repoID)
	if err != nil {
		return errors.Wrapf(err, "finding original repo '%s'", h.repoID)
	}
	if oldRepoRef == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("repo '%s' not found", h.repoID),
		}
	}

	requestProjectRef := &model.APIProjectRef{}
	if err = requestProjectRef.BuildFromService(ctx, oldRepoRef.ProjectRef); err != nil {
		return errors.Wrap(err, "converting original repo to API model")
	}

	// Erase so apiNewRepoRef only tracks new additions.
	requestProjectRef.Admins = nil
	requestProjectRef.GitTagAuthorizedUsers = nil
	requestProjectRef.GitTagAuthorizedTeams = nil

	if err = json.Unmarshal(b, requestProjectRef); err != nil {
		return errors.Wrap(err, "unmarshalling modified repo settings")
	}

	if projectId := utility.FromStringPtr(requestProjectRef.Id); projectId != oldRepoRef.Id {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "repo ID is immutable",
		}
	}

	newProjectRef, err := requestProjectRef.ToService()
	if err != nil {
		return errors.Wrap(err, "converting new repo to service model")
	}

	h.newRepoRef = &dbModel.RepoRef{ProjectRef: *newProjectRef}
	h.originalRepoRef = oldRepoRef
	h.apiNewRepoRef = requestProjectRef
	return nil
}

func (h *repoIDPatchHandler) Run(ctx context.Context) gimlet.Responder {
	if err := h.newRepoRef.ValidateOwnerAndRepo(h.settings.GithubOrgs); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "validating owner and repo"))
	}

	before, err := dbModel.GetProjectSettings(ctx, &h.originalRepoRef.ProjectRef)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting original repo settings for repo '%s'", h.repoID))
	}

	adminsToDelete := utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteAdmins)
	adminsToAdd := h.newRepoRef.Admins
	h.newRepoRef.Admins = mergeListWithDeletions(h.originalRepoRef.Admins, h.newRepoRef.Admins, adminsToDelete)
	h.newRepoRef.GitTagAuthorizedUsers = mergeListWithDeletions(h.originalRepoRef.GitTagAuthorizedUsers, h.newRepoRef.GitTagAuthorizedUsers, utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteGitTagAuthorizedUsers))
	h.newRepoRef.GitTagAuthorizedTeams = mergeListWithDeletions(h.originalRepoRef.GitTagAuthorizedTeams, h.newRepoRef.GitTagAuthorizedTeams, utility.FromStringPtrSlice(h.apiNewRepoRef.DeleteGitTagAuthorizedTeams))

	if resp := validateProjectRefSettings(ctx, &h.newRepoRef.ProjectRef); resp != nil {
		return resp
	}

	if err = h.newRepoRef.Replace(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating repo '%s'", h.repoID))
	}

	if err = h.newRepoRef.UpdateAdminRoles(ctx, adminsToAdd, adminsToDelete); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "updating admins for repo '%s'", h.repoID))
	}

	if resp := saveProjectOrRepoSettings(ctx, &h.newRepoRef.ProjectRef, h.apiNewRepoRef, h.user.Username(), before); resp != nil {
		return resp
	}

	return gimlet.NewJSONResponse(struct{}{})
}
