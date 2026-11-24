package route

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restData "github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// POST /patches/{patch_id}/clone

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
//	@Description	Creates a new patch copying the diffs, base commit, description, and task/variant definition from the given patch. The clone is created unactivated unless 'finalize' is true. The caller or author must have 'Patches: Submit and Edit' permission on the project (and all child projects if relevant). Merge queue patches cannot be cloned, since their base branch is temporary.
//	@Tags			patches
//	@Router			/patches/{patch_id}/clone [post]
//	@Security		Api-User || Api-Key
//	@Param			patch_id	path		string						true	"the patch ID to clone"
//	@Param			{object}	body		apimodels.ClonePatchRequest	false	"optional overrides"
//	@Success		201			{object}	model.APIPatch
//	@Failure		400			{object}	gimlet.ErrorResponse	"the patch cannot be cloned or has nothing to finalize"
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
	b, err := io.ReadAll(r.Body)
	if err != nil {
		return errors.Wrap(err, "reading clone options from request body")
	}
	if len(b) > 0 {
		if err = json.Unmarshal(b, &h.body); err != nil {
			return errors.Wrap(err, "reading clone options from JSON request body")
		}
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
	// A merge queue patch's base branch is temporary, so the clone could never run.
	if source.IsMergeQueuePatch() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "merge queue patches cannot be cloned",
		})
	}

	// Child patches are internal artifacts of the trigger alias flow; cloning the parent patch reproduces them.
	if source.IsChild() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "child patches cannot be cloned directly; clone the parent patch instead",
		})
	}

	// Patches with very large diffs only store patch summaries, not their diff contents, so a clone would not
	// reproduce the source's changes.
	for _, p := range source.Patches {
		if p.PatchSet.PatchFileId == "" && p.PatchSet.Patch == "" && len(p.PatchSet.Summary) > 0 {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("patch '%s' cannot be cloned because its diff contents are not stored", h.patchID),
			})
		}
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

	// The base diff is the patch entry with no module name; module diffs keep their own githashes.
	raw, err := restData.GetRawPatches(ctx, h.patchID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	var modules []apimodels.CreatePatchModule
	for _, m := range raw.RawModules {
		modules = append(modules, apimodels.CreatePatchModule{
			ModuleName: m.Name,
			Githash:    m.Githash,
			Patch:      m.Diff,
		})
	}

	description := source.Description
	if h.body.Description != "" {
		description = h.body.Description
	}

	// Unfinalized sources have no activated tasks for the repeat logic to read, so their resolved definition is
	// copied onto the clone directly after processing.
	var definition []patch.VariantTasks
	if source.Version == "" {
		definition = source.VariantsTasks
	}

	return createPatchFromIntent(ctx, h.env, usr, createPatchArgs{
		pref:          pref,
		author:        h.body.Author,
		intentParams:  cloneIntentParams(source, raw, description),
		variantsTasks: definition,
		modules:       modules,
		finalize:      h.body.Finalize,
	})
}

// cloneIntentParams builds the intent for a cloned patch, copying the source patch's diffs and definition. Trigger
// aliases are copied too, so cloning a patch with child patches reproduces them; each child project's patch
// submission permission is checked when the processor spawns the child patches.
func cloneIntentParams(source *patch.Patch, raw *restModel.APIRawPatch, description string) patch.CLIIntentParams {
	params := patch.CLIIntentParams{
		BaseGitHash:                        source.Githash,
		PatchContent:                       raw.Patch.Diff,
		Path:                               source.Path,
		Description:                        description,
		Parameters:                         source.Parameters,
		TriggerAliases:                     source.Triggers.Aliases,
		LocalModuleIncludes:                source.LocalModuleIncludes,
		RegexVariants:                      source.RegexBuildVariants,
		RegexTasks:                         source.RegexTasks,
		RegexTestSelectionVariants:         source.RegexTestSelectionBuildVariants,
		RegexTestSelectionExcludedVariants: source.RegexTestSelectionExcludedBuildVariants,
		RegexTestSelectionTasks:            source.RegexTestSelectionTasks,
		RegexTestSelectionExcludedTasks:    source.RegexTestSelectionExcludedTasks,
	}
	if source.Version != "" {
		// Reuse the source patch's exact build variant/task selection through the repeat logic rather than copying
		// its variants and tasks separately, which would lose the per-variant task mapping.
		params.RepeatDefinition = true
		params.RepeatPatchId = source.Id.Hex()
	}
	return params
}
