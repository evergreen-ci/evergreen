package route

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restData "github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// POST /projects/{project_id}/patches
//
// Creates a patch in one call: base diff, module diffs, task/variant
// definition, and optional finalization. The base diff and module diffs are
// all optional, so a patch with no diffs runs its tasks against the base
// commit.
type patchCreateHandler struct {
	env       evergreen.Environment
	projectID string
	body      apimodels.CreatePatchRequest
}

func makeCreatePatchHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &patchCreateHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Create a patch
//	@Description	Creates a patch for the project. The base diff ('patch') and module diffs are optional; a patch with no diffs runs its tasks against the base commit. Module diffs are given inline, so no follow-up calls are needed. To repeat a previous patch's build variant/task definition, set exactly one of 'repeat_last_patch', 'repeat_failed', or 'repeat_patch_id' instead of specifying variants and tasks. Returns the created patch; it is not activated unless 'finalize' is true.
//	@Tags			patches
//	@Router			/projects/{project_id}/patches [post]
//	@Security		Api-User || Api-Key
//	@Param			project_id	path		string							true	"the project ID or identifier"
//	@Param			{object}	body		apimodels.CreatePatchRequest	true	"patch to create"
//	@Success		201			{object}	model.APIPatch
//	@Failure		400			{object}	gimlet.ErrorResponse	"invalid request, such as an unknown module or no tasks/variants to finalize"
func (h *patchCreateHandler) Factory() gimlet.RouteHandler {
	return &patchCreateHandler{env: h.env}
}

func (h *patchCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	h.projectID = gimlet.GetVars(r)["project_id"]
	if h.projectID == "" {
		return errors.New("missing project ID")
	}

	body := utility.NewRequestReaderWithSize(r, patch.SizeLimit)
	defer body.Close()
	if err := utility.ReadJSON(body, &h.body); err != nil {
		return errors.Wrap(err, "reading patch from JSON request body")
	}
	return h.body.Validate()
}

func (h *patchCreateHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)

	pref, err := dbModel.FindMergedProjectRef(ctx, h.projectID, "", true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", h.projectID))
	}
	if pref == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", h.projectID),
		})
	}
	if pref.IsPatchingDisabled() || !pref.Enabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("patching is disabled for project '%s'", pref.Identifier),
		})
	}

	author, errResp := resolvePatchAuthor(ctx, usr, pref, h.body.Author)
	if errResp != nil {
		return errResp
	}

	parameters := make([]patch.Parameter, 0, len(h.body.Parameters))
	for _, p := range h.body.Parameters {
		parameters = append(parameters, patch.Parameter{Key: p.Key, Value: p.Value})
	}

	patchID := mgobson.NewObjectId()
	intent, err := patch.NewCliIntent(patch.CLIIntentParams{
		User:                               author,
		Project:                            pref.Id,
		BaseGitHash:                        h.body.BaseGithash,
		PatchContent:                       h.body.Patch,
		Description:                        h.body.Description,
		Finalize:                           false,
		Parameters:                         parameters,
		Variants:                           h.body.Variants,
		Tasks:                              h.body.Tasks,
		RegexVariants:                      h.body.RegexVariants,
		RegexTasks:                         h.body.RegexTasks,
		RegexTestSelectionVariants:         h.body.RegexTestSelectionVariants,
		RegexTestSelectionExcludedVariants: h.body.RegexTestSelectionExcludedVariants,
		RegexTestSelectionTasks:            h.body.RegexTestSelectionTasks,
		RegexTestSelectionExcludedTasks:    h.body.RegexTestSelectionExcludedTasks,
		Alias:                              h.body.Alias,
		Aliases:                            h.body.Aliases,
		TriggerAliases:                     h.body.TriggerAliases,
		RepeatDefinition:                   h.body.RepeatLastPatch,
		RepeatFailed:                       h.body.RepeatFailed,
		RepeatPatchId:                      h.body.RepeatPatchId,
	})
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}
	if intent == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "intent could not be created from supplied data",
		})
	}
	if err = intent.Insert(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "inserting patch intent"))
	}

	grip.Info(ctx, message.Fields{
		"operation":  "patch creation",
		"message":    "creating patch",
		"from":       "rest v2",
		"patch_id":   patchID,
		"finalizing": h.body.Finalize,
		"variants":   h.body.Variants,
		"tasks":      h.body.Tasks,
		"alias":      h.body.Alias,
	})
	job := units.NewPatchIntentProcessor(h.env, patchID, intent)
	job.Run(ctx)
	if err = job.Error(); err != nil {
		// Return a 400 error if the error is due to the user's input.
		if strings.Contains(err.Error(), units.BuildTasksAndVariantsError) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "processing patch"))
	}

	// Module diffs are applied after the patch doc exists; they must be in
	// place before finalization so the version picks them up.
	for _, m := range h.body.Modules {
		if err = restData.UpdatePatchModule(ctx, patchID.Hex(), m.ModuleName, m.Githash, m.Patch); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding diff for module '%s'", m.ModuleName))
		}
	}

	if h.body.Finalize {
		if err = restData.SetPatchActivated(ctx, patchID.Hex(), usr.Id, true); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "finalizing patch"))
		}
	}

	apiPatch, err := restData.FindPatchById(ctx, patchID.Hex())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", patchID.Hex()))
	}

	responder := gimlet.NewJSONResponse(apiPatch)
	if err := responder.SetStatus(http.StatusCreated); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "setting response status"))
	}
	return responder
}

// resolvePatchAuthor returns the user the patch should be attributed to: the
// caller, or the requested author if the caller has admin patch permissions.
// Matching the CLI, an author who does not exist falls back to the caller.
func resolvePatchAuthor(ctx context.Context, usr *user.DBUser, pref *dbModel.ProjectRef, author string) (string, gimlet.Responder) {
	if author == "" || author == usr.Id {
		return usr.Id, nil
	}

	if !usr.HasPermission(ctx, gimlet.PermissionOpts{
		Resource:      pref.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionPatches,
		RequiredLevel: evergreen.PatchSubmitAdmin.Value,
	}) {
		return "", gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusForbidden,
			Message:    "user is not authorized to patch on behalf of other users",
		})
	}

	specified, err := user.FindOneById(ctx, author)
	if err != nil {
		return "", gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding author '%s'", author))
	}
	if specified != nil {
		return specified.Id, nil
	}
	return usr.Id, nil
}
