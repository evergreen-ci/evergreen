package apiserver

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/patch"
	"10gen.com/mci/thirdparty"
	"fmt"
	"github.com/gorilla/mux"
	"labix.org/v2/mgo/bson"
	"net/http"
	"strings"
	"time"
)

// Get the patch with the specified request it
func getPatchFromRequest(r *http.Request) (*model.Patch, error) {
	// get id and secret from the request.
	vars := mux.Vars(r)
	patchId := vars["patchId"]
	if len(patchId) == 0 {
		return nil, fmt.Errorf("no patch id supplied")
	}

	// find the patch
	existingPatch, err := model.FindExistingPatch(patchId)

	if existingPatch == nil {
		return nil, fmt.Errorf("no existing request with id: %v", patchId)
	}

	if err != nil {
		return nil, err
	}
	return existingPatch, nil
}

func (as *APIServer) submitPatch(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	if !getGlobalLock(PatchLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Failed to get global lock"))
		return
	}
	defer releaseGlobalLock(PatchLockTitle)

	apiRequest := model.PatchAPIRequest{
		ProjectFileName: r.FormValue("project"),
		ModuleName:      r.FormValue("module"),
		Githash:         r.FormValue("githash"),
		PatchContent:    r.FormValue("patch"),
		BuildVariants:   strings.Split(r.FormValue("buildvariants"), ","),
	}

	description := r.FormValue("desc")
	projId := r.FormValue("project")
	project, err := model.FindProject("", projId, as.MCISettings.ConfigDir)
	if err != nil {
		message := fmt.Errorf("Error locating project '%v' from '%v': %v",
			projId, as.MCISettings.ConfigDir, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if project == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("project %v not found", projId))
		return
	}

	patchMetadata, message, err := apiRequest.Validate(as.MCISettings.ConfigDir, as.MCISettings.Credentials[project.RepoKind])
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Invalid patch: %v - %v", message, err))
		return
	}

	if patchMetadata == nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("patch metadata is empty"))
		return
	}

	if apiRequest.ModuleName != "" {
		as.WriteJSON(w, http.StatusBadRequest, "module not allowed when creating new patches (must be added in a subsequent request)")
		return
	}

	commitInfo, err := thirdparty.GetCommitEvent(as.MCISettings.Credentials[project.RepoKind],
		patchMetadata.Project.Owner,
		patchMetadata.Project.Repo,
		apiRequest.Githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if commitInfo == nil {
		as.WriteJSON(w, http.StatusBadRequest, "That commit doesn't seem to exist.")
		return
	}

	//Check if the user already has some patch on this same commit+project
	_, err = model.FindPatchByUserProjectGitspec(user.Id, apiRequest.ProjectFileName, apiRequest.Githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	createTime := time.Now()
	patchDoc := &model.Patch{
		Id:            bson.NewObjectId(),
		Description:   description,
		Author:        user.Id,
		Project:       apiRequest.ProjectFileName,
		Githash:       apiRequest.Githash,
		CreateTime:    createTime,
		Status:        mci.PatchCreated,
		BuildVariants: apiRequest.BuildVariants,
		Tasks:         nil, // nil : ALL tasks. non-nil: compile + any tasks included in list.
		Patches: []model.ModulePatch{
			model.ModulePatch{
				ModuleName: "",
				Githash:    apiRequest.Githash,
				PatchSet: model.PatchSet{
					Patch:   apiRequest.PatchContent,
					Summary: patchMetadata.Summaries, // thirdparty.GetPatchSummary(apiRequest.PatchContent),
				},
			},
		},
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = patchDoc.ComputePatchNumber()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error computing patch num %v", err))
		return
	}

	err = patchDoc.Insert()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error inserting patch: %v", err))
		return
	}

	if strings.ToLower(r.FormValue("finalize")) == "true" {
		if _, err = patch.Finalize(patchDoc, &as.MCISettings); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	as.WriteJSON(w, http.StatusCreated, patch.PatchAPIResponse{Patch: patchDoc})
}

func (as *APIServer) updatePatchModule(w http.ResponseWriter, r *http.Request) {
	//TODO log any instances of err != nil (internal server error)
	p, err := getPatchFromRequest(r)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	moduleName := r.FormValue("module")
	patchContent := r.FormValue("patch")
	githash := r.FormValue("githash")

	project, err := model.FindProject("", p.Project, as.MCISettings.ConfigDir)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error getting patch: %v", err))
		return
	}
	if project == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("can't find project: %v", p.Project))
		return
	}

	module, err := project.GetModuleByName(moduleName)
	if err != nil || module == nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("No such module"))
		return
	}

	gitOutput, err := thirdparty.GitApplyNumstat(patchContent)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Errorf("Invalid patch: %v", err))
		return
	}
	if gitOutput == nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Errorf("Empty diff"))
		return
	}

	summaries, err := thirdparty.ParseGitSummary(gitOutput)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Errorf("Can't validate patch: %v", err))
		return
	}

	repoOwner, repo := module.GetRepoOwnerAndName()

	commitInfo, err := thirdparty.GetCommitEvent(as.MCISettings.Credentials[project.RepoKind], repoOwner, repo, githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if commitInfo == nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Errorf("commit hash doesn't seem to exist"))
		return
	}

	if !getGlobalLock(PatchLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(PatchLockTitle)

	//Things look ok - go ahead and add/update the module patch
	modulePatch := model.ModulePatch{
		ModuleName: moduleName,
		Githash:    githash,
		PatchSet: model.PatchSet{
			Patch:   patchContent,
			Summary: summaries, // thirdparty.GetPatchSummary(apiRequest.PatchContent),
		},
	}
	err = p.UpdateModulePatch(modulePatch)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, "Patch module updated")
	return
}

func (as *APIServer) listPatches(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	patches, err := model.FindPatchesByUser(user.Id, []string{}, 0, 0)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			fmt.Errorf("error finding patches for user %v: %v", user.Id, err))
		return
	}
	as.WriteJSON(w, http.StatusOK, patches)
}

func (as *APIServer) existingPatchRequest(w http.ResponseWriter, r *http.Request) {
	// get patch from request
	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if !getGlobalLock(PatchLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(PatchLockTitle)
	// dispatch to handlers based on specified action
	switch r.FormValue("action") {
	case "update":
		name := r.FormValue("desc")
		err := p.SetDescription(name)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		as.WriteJSON(w, http.StatusOK, "patch updated")
	case "finalize":
		if p.Activated == true {
			http.Error(w, "patch is already finalized", http.StatusBadRequest)
			return
		}
		_, err = patch.Finalize(p, &as.MCISettings)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		as.WriteJSON(w, http.StatusOK, "patch finalized")
	case "cancel":
		err = p.Cancel()
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		as.WriteJSON(w, http.StatusOK, "patch deleted")
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", r.FormValue("action")), http.StatusBadRequest)
	}
}

func (as *APIServer) summarizePatch(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	as.WriteJSON(w, http.StatusOK, patch.PatchAPIResponse{Patch: p})
}

func (as *APIServer) deletePatchModule(w http.ResponseWriter, r *http.Request) {
	if !getGlobalLock(PatchLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("timed out taking global lock"))
		return
	}
	defer releaseGlobalLock(PatchLockTitle)

	p, err := getPatchFromRequest(r)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}
	moduleName := r.FormValue("module")
	if moduleName == "" {
		as.WriteJSON(w, http.StatusBadRequest, "You must specify a module to delete")
		return
	}

	// don't mess with already finalized requests
	if p.Activated {
		response := fmt.Sprintf("Can't delete module - path already finalized")
		as.WriteJSON(w, http.StatusBadRequest, response)
		return
	}

	err = p.RemoveModulePatch(moduleName)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, patch.PatchAPIResponse{Message: "module removed from patch."})
}
