package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restData "github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// POST /patches/{patch_id}/clone
//
// Creates a new patch copying the diffs, base commit, and task/variant definition from an existing patch, replacing
// the CLI's `patch-file --diff-patchId` flow, which faked cloning by replaying a source patch's raw diffs.
type patchCloneHandler struct {
	env     evergreen.Environment
	patchID string
	body    apimodels.ClonePatchRequest
}

func makeClonePatchHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &patchCloneHandler{env: env}
}

// Factory creates an instance of the handler.
//
//	@Summary		Clone a patch
//	@Description	Creates a new patch copying the diffs, base commit, description, and task/variant definition from the given patch. The request body is optional and can override the description and finalize the clone. The clone is created unactivated unless 'finalize' is true. Cloning a patch with trigger aliases reproduces its child patches in their projects, which requires 'Patches: Submit and Edit' permission on each child project. The caller must have 'Patches: Submit and Edit' permission in the source patch's project; the clone belongs to the caller in the same project.
//	@Tags			patches
//	@Router			/patches/{patch_id}/clone [post]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string						true	"the patch ID to clone"
//	@Param			{object}	body		apimodels.ClonePatchRequest	false	"optional overrides"
//	@Success		201			{object}	model.APIPatch
//	@Failure		400			{object}	gimlet.ErrorResponse	"the source patch has no tasks or variants to finalize"
func (h *patchCloneHandler) Factory() gimlet.RouteHandler {
	return &patchCloneHandler{env: h.env}
}

func (h *patchCloneHandler) Parse(ctx context.Context, r *http.Request) error {
	h.patchID = gimlet.GetVars(r)["patch_id"]
	if h.patchID == "" {
		return errors.New("missing patch ID")
	}
	if err := restData.ValidatePatchID(h.patchID); err != nil {
		return err
	}

	// The body is optional.
	if r.Body == nil || r.ContentLength == 0 {
		return nil
	}
	body := utility.NewRequestReader(r)
	defer body.Close()
	if err := utility.ReadJSON(body, &h.body); err != nil {
		return errors.Wrap(err, "reading clone options from JSON request body")
	}
	return nil
}

func (h *patchCloneHandler) Run(ctx context.Context) gimlet.Responder {
	usr := MustHaveUser(ctx)

	source, err := patch.FindOneId(ctx, h.patchID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding patch '%s'", h.patchID))
	}
	if source == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("patch '%s' not found", h.patchID),
		})
	}
	if err = source.FetchPatchFiles(ctx); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "fetching diff files for patch '%s'", h.patchID))
	}

	if h.body.Finalize && len(source.VariantsTasks) == 0 &&
		len(source.BuildVariants)+len(source.RegexBuildVariants)+len(source.Tasks)+len(source.RegexTasks) == 0 &&
		source.Alias == "" && len(source.Aliases) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot finalize a clone of a patch with no tasks or variants",
		})
	}

	description := source.Description
	if h.body.Description != "" {
		description = h.body.Description
	}

	pref, err := dbModel.FindMergedProjectRef(ctx, source.Project, "", true)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding project '%s'", source.Project))
	}
	if pref == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("project '%s' not found", source.Project),
		})
	}

	// The base diff is the patch entry with no module name; module diffs keep
	// their own githashes.
	raw, err := restData.GetRawPatches(ctx, h.patchID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting diffs for patch '%s'", h.patchID))
	}
	var modules []apimodels.CreatePatchModule
	for _, m := range raw.RawModules {
		modules = append(modules, apimodels.CreatePatchModule{
			ModuleName: m.Name,
			Githash:    m.Githash,
			Patch:      m.Diff,
		})
	}

	return createPatchFromIntent(ctx, h.env, usr, createPatchArgs{
		pref:         pref,
		author:       h.body.Author,
		intentParams: cloneIntentParams(source, raw, description),
		modules:      modules,
		finalize:     h.body.Finalize,
	})
}

// cloneIntentParams builds the intent for a cloned patch, copying the source patch's diffs and definition. Trigger
// aliases are copied too, so cloning a patch with child patches reproduces them; each child project's patch
// submission permission is checked when the processor spawns the child patches.
func cloneIntentParams(source *patch.Patch, raw *restModel.APIRawPatch, description string) patch.CLIIntentParams {
	return patch.CLIIntentParams{
		BaseGitHash:                        source.Githash,
		PatchContent:                       raw.Patch.Diff,
		Path:                               source.Path,
		Description:                        description,
		Parameters:                         source.Parameters,
		Variants:                           source.BuildVariants,
		Tasks:                              source.Tasks,
		RegexVariants:                      source.RegexBuildVariants,
		RegexTasks:                         source.RegexTasks,
		RegexTestSelectionVariants:         source.RegexTestSelectionBuildVariants,
		RegexTestSelectionExcludedVariants: source.RegexTestSelectionExcludedBuildVariants,
		RegexTestSelectionTasks:            source.RegexTestSelectionTasks,
		RegexTestSelectionExcludedTasks:    source.RegexTestSelectionExcludedTasks,
		Alias:                              source.Alias,
		Aliases:                            source.Aliases,
		TriggerAliases:                     source.Triggers.Aliases,
		LocalModuleIncludes:                source.LocalModuleIncludes,
		CrossFileYAMLAnchorsEnabled:        source.CrossFileYAMLAnchorsEnabled,
	}
}
