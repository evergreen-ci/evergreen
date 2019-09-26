package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const formMimeType = "application/x-www-form-urlencoded"

// PatchAPIResponse is returned by all patch-related API calls
type PatchAPIResponse struct {
	Message string       `json:"message"`
	Action  string       `json:"action"`
	Patch   *patch.Patch `json:"patch"`
}

// submitPatch creates the Patch document, adds the patched project config to it,
// and saves the patches to GridFS to be retrieved
func (as *APIServer) submitPatch(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)

	data := struct {
		Description string   `json:"desc"`
		Project     string   `json:"project"`
		Patch       string   `json:"patch"`
		Githash     string   `json:"githash"`
		Variants    string   `json:"buildvariants"`
		Tasks       []string `json:"tasks"`
		Finalize    bool     `json:"finalize"`
		Alias       string   `json:"alias"`
		CommitQueue bool     `json:"commit_queue"`
	}{}
	if err := util.ReadJSONInto(util.NewRequestReaderWithSize(r, patch.SizeLimit), &data); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}
	if len(data.Patch) > patch.SizeLimit {
		as.LoggedError(w, r, http.StatusBadRequest, errors.New("Patch is too large"))
		return
	}
	variants := strings.Split(data.Variants, ",")

	pref, err := model.FindOneProjectRef(data.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrapf(err, "project %s is not specified", data.Project))
		return
	}
	if pref == nil {
		gimlet.WriteJSONResponse(w, http.StatusNotFound,
			gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("project '%s' is not found", data.Project),
			})
		return
	}

	if pref.PatchingDisabled || !pref.Enabled {
		as.LoggedError(w, r, http.StatusUnauthorized, errors.New("patching is disabled"))
		return
	}

	intent, err := patch.NewCliIntent(dbUser.Id, data.Project, data.Githash, r.FormValue("module"), data.Patch, data.Description, data.Finalize, variants, data.Tasks, data.Alias)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	if intent == nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.New("intent could not be created from supplied data"))
		return
	}
	if err = intent.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	patchID := mgobson.NewObjectId()
	job := units.NewPatchIntentProcessor(patchID, intent)
	job.Run(r.Context())

	if err = job.Error(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error processing patch"))
		return
	}

	patchDoc, err := patch.FindOne(patch.ById(patchID))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.New("can't fetch patch data"))
		return
	}
	if patchDoc == nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.New("patch couldn't be found"))
		return
	}

	gimlet.WriteJSONResponse(w, http.StatusCreated, PatchAPIResponse{Patch: patchDoc})
}

// Get the patch with the specified request it
func getPatchFromRequest(r *http.Request) (*patch.Patch, error) {
	// get id and secret from the request.
	patchIdStr := gimlet.GetVars(r)["patchId"]
	if len(patchIdStr) == 0 {
		return nil, errors.New("no patch id supplied")
	}
	if !patch.IsValidId(patchIdStr) {
		return nil, errors.Errorf("patch id '%v' is not valid object id", patchIdStr)
	}

	// find the patch
	existingPatch, err := patch.FindOne(patch.ById(patch.NewId(patchIdStr)))
	if err != nil {
		return nil, err
	}
	if existingPatch == nil {
		return nil, errors.Errorf("no existing request with id: %v", patchIdStr)
	}
	return existingPatch, nil
}

func (as *APIServer) updatePatchModule(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		gimlet.WriteJSONError(w, err.Error())
		return
	}

	githubOauthToken, err := as.Settings.GetGithubOauthToken()
	if err != nil {
		gimlet.WriteJSONError(w, err)
		return
	}

	var moduleName, patchContent, githash, message string

	if r.Header.Get("Content-Type") == formMimeType {
		moduleName = r.FormValue("module")
		patchContent = r.FormValue("patch")
		githash = r.FormValue("githash")
		message = r.FormValue("message")
	} else {
		data := struct {
			Module  string `json:"module"`
			Patch   string `json:"patch"`
			Githash string `json:"githash"`
			Message string `json:"message"`
		}{}
		if err = util.ReadJSONInto(util.NewRequestReader(r), &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		moduleName, patchContent, githash, message = data.Module, data.Patch, data.Githash, data.Message
	}

	projectRef, err := model.FindOneProjectRef(p.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "Error getting project ref with id %v", p.Project))
		return
	}
	project, err := model.FindLastKnownGoodProject(projectRef.Identifier)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error getting patch"))
		return
	}
	if project == nil {
		as.LoggedError(w, r, http.StatusNotFound, errors.Errorf("can't find project: %v", p.Project))
		return
	}

	module, err := project.GetModuleByName(moduleName)
	if err != nil || module == nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("No such module: %s", moduleName))
		return
	}

	summaries, err := thirdparty.GetPatchSummaries(patchContent)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	repoOwner, repo := module.GetRepoOwnerAndName()

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	_, err = thirdparty.GetCommitEvent(ctx, githubOauthToken, repoOwner, repo, githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// write the patch content into a GridFS file under a new ObjectId.
	patchFileId := mgobson.NewObjectId().Hex()
	err = db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(patchContent))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "failed to write patch file to db"))
		return
	}

	modulePatch := patch.ModulePatch{
		ModuleName: moduleName,
		Message:    message,
		Githash:    githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileId,
			Summary:     summaries,
		},
	}

	if p.Version != "" && p.Alias == evergreen.CommitQueueAlias {
		as.LoggedError(w, r, http.StatusBadRequest, errors.New("can't update modules for in-flight commit queue tests"))
		return
	}

	if err = p.UpdateModulePatch(modulePatch); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if p.Version != "" {
		patchVersion, err := model.VersionFindOneId(p.Version)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding patch version %s", p.Version))
			return
		}
		m, err := manifest.FindFromVersion(patchVersion.Id, patchVersion.Identifier, patchVersion.Revision)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "error finding patch manifest"))
			return
		}
		err = m.UpdateModuleRevision(moduleName, githash)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding updating revision for module %s", moduleName))
			return
		}
	}

	gimlet.WriteJSON(w, "Patch module updated")
}

// listPatches returns a user's "n" most recent patches.
func (as *APIServer) listPatches(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)
	n, err := util.GetIntValue(r, "n", 0)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "cannot read value n"))
		return
	}
	query := patch.ByUser(dbUser.Id).Sort([]string{"-" + patch.CreateTimeKey})
	if n > 0 {
		query = query.Limit(n)
	}
	patches, err := patch.Find(query)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "error finding patches for user %s", dbUser.Id))
		return
	}
	gimlet.WriteJSON(w, patches)
}

func (as *APIServer) existingPatchRequest(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var action, desc string
	if r.Header.Get("Content-Type") == formMimeType {
		action = r.FormValue("action")
	} else {
		data := struct {
			PatchId     string `json:"patch_id"`
			Action      string `json:"action"`
			Description string `json:"description"`
		}{}
		if err = util.ReadJSONInto(util.NewRequestReader(r), &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		action, desc = data.Action, data.Description
	}

	// dispatch to handlers based on specified action
	switch action {
	case "update":
		err = p.SetDescription(desc)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		gimlet.WriteJSON(w, "patch updated")
	case "finalize":
		var githubOauthToken string
		githubOauthToken, err = as.Settings.GetGithubOauthToken()
		if err != nil {
			gimlet.WriteJSONInternalError(w, err)
			return
		}

		if p.Activated {
			http.Error(w, "patch is already finalized", http.StatusBadRequest)
			return
		}

		_, projectYaml, err := model.GetPatchedProject(ctx, p, githubOauthToken)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		p.PatchedConfig = projectYaml
		_, err = model.FinalizePatch(ctx, p, evergreen.PatchVersionRequester, githubOauthToken)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		gimlet.WriteJSON(w, "patch finalized")
	case "cancel":
		err = model.CancelPatch(p, dbUser.Id)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		gimlet.WriteJSON(w, "patch deleted")
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", action), http.StatusBadRequest)
	}
}

func (as *APIServer) summarizePatch(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	gimlet.WriteJSON(w, PatchAPIResponse{Patch: p})
}

func (as *APIServer) listPatchModules(w http.ResponseWriter, r *http.Request) {
	_, project := MustHaveProject(r)

	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data := struct {
		Project string   `json:"project"`
		Modules []string `json:"modules"`
	}{
		Project: project.Identifier,
	}

	mods := map[string]struct{}{}

	for _, m := range project.Modules {
		if m.Name == "" {
			continue
		}
		mods[m.Name] = struct{}{}
	}

	for _, m := range p.Patches {
		mods[m.ModuleName] = struct{}{}
	}

	for m := range mods {
		data.Modules = append(data.Modules, m)
	}

	gimlet.WriteJSON(w, &data)
}

func (as *APIServer) deletePatchModule(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}
	moduleName := r.FormValue("module")
	if moduleName == "" {
		gimlet.WriteJSONError(w, "You must specify a module to delete")
		return
	}

	// don't mess with already finalized requests
	if p.Activated {
		response := fmt.Sprintf("Can't delete module - path already finalized")
		gimlet.WriteJSONError(w, response)
		return
	}

	err = p.RemoveModulePatch(moduleName)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, PatchAPIResponse{Message: "module removed from patch."})
}
