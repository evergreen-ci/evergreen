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
//	@Description	Creates a patch for the project. The base diff ('patch') and module diffs are optional; a patch with no diffs runs its tasks against the base commit. Module diffs are given inline, so no follow-up calls are needed. To repeat a previous patch's build variant/task definition, set exactly one of 'repeat_last_patch', 'repeat_failed', or 'repeat_patch_id' instead of specifying variants and tasks. Returns the created patch; it is not activated unless 'finalize' is true. Patch creation runs synchronously and can take time, so clients should use a generous request timeout and must not blindly retry: a retried request that already succeeded creates a duplicate patch.
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

	parameters := make([]patch.Parameter, 0, len(h.body.Parameters))
	for _, p := range h.body.Parameters {
		parameters = append(parameters, patch.Parameter{Key: p.Key, Value: p.Value})
	}

	return createPatchFromIntent(ctx, h.env, usr, createPatchArgs{
		pref:   pref,
		author: h.body.Author,
		intentParams: patch.CLIIntentParams{
			BaseGitHash:                        h.body.BaseGithash,
			PatchContent:                       h.body.Patch,
			Description:                        h.body.Description,
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
		},
		modules:  h.body.Modules,
		finalize: h.body.Finalize,
	})
}

// createPatchArgs holds everything the shared patch creation flow needs. The
// author, project, and finalize state on the intent params are set by
// createPatchFromIntent.
type createPatchArgs struct {
	pref         *dbModel.ProjectRef
	author       string
	intentParams patch.CLIIntentParams
	modules      []apimodels.CreatePatchModule
	finalize     bool
}

// createPatchFromIntent is the shared creation flow behind the create and
// clone routes: validate the project, insert and process a CLI intent, apply
// module diffs, and optionally finalize. It returns a responder with the
// created APIPatch (201), or an error responder.
func createPatchFromIntent(ctx context.Context, env evergreen.Environment, usr *user.DBUser, args createPatchArgs) gimlet.Responder {
	pref := args.pref
	if pref.IsPatchingDisabled() || !pref.Enabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("patching is disabled for project '%s'", pref.Identifier),
		})
	}

	author, errResp := resolvePatchAuthor(ctx, usr, pref, args.author)
	if errResp != nil {
		return errResp
	}

	args.intentParams.User = author
	args.intentParams.Project = pref.Id
	args.intentParams.Finalize = false

	patchID := mgobson.NewObjectId()
	intent, err := patch.NewCliIntent(args.intentParams)
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
		"finalizing": args.finalize,
		"variants":   args.intentParams.Variants,
		"tasks":      args.intentParams.Tasks,
		"alias":      args.intentParams.Alias,
	})
	job := units.NewPatchIntentProcessor(env, patchID, intent)
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

	// The processed patch doc is needed to validate modules and to check that finalization is meaningful. Module diffs
	// must be in place before finalization so the version picks them up.
	if len(args.modules) > 0 || args.finalize {
		patchDoc, err := patch.FindOneId(ctx, patchID.Hex())
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", patchID.Hex()))
		}
		if patchDoc == nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("patch '%s' not found after processing", patchID.Hex()))
		}

		if errResp := validateModulesExist(ctx, env, patchDoc, args.modules); errResp != nil {
			return errResp
		}

		// Activating a patch whose definition resolved to no tasks or variants would create a version with nothing to run.
		if args.finalize && len(patchDoc.VariantsTasks) == 0 {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "patch has no build variants or tasks",
			})
		}
	}

	for _, m := range args.modules {
		if err = restData.UpdatePatchModule(ctx, patchID.Hex(), m.ModuleName, m.Githash, m.Patch); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding diff for module '%s'", m.ModuleName))
		}
	}

	if args.finalize {
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

// validateModulesExist rejects unknown module names with a 400 so they block creation instead of silently attaching a
// diff no build can use.
func validateModulesExist(ctx context.Context, env evergreen.Environment, patchDoc *patch.Patch, modules []apimodels.CreatePatchModule) gimlet.Responder {
	if len(modules) == 0 {
		return nil
	}
	patchedProject, _, err := dbModel.GetPatchedProject(ctx, env.Settings(), patchDoc)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting patched project to validate modules"))
	}
	for _, m := range modules {
		if _, err = patchedProject.GetModuleByName(m.ModuleName); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Wrapf(err, "module '%s'", m.ModuleName).Error(),
			})
		}
	}
	return nil
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
