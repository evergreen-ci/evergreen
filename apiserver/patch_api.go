package apiserver

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
	"net/http"
	"strings"
	"time"
)

// PatchAPIResponse is returned by all patch-related API calls
type PatchAPIResponse struct {
	Message string       `json:"message"`
	Action  string       `json:"action"`
	Patch   *patch.Patch `json:"patch"`
}

// PatchAPIRequest in the input struct with which we process patch requests
type PatchAPIRequest struct {
	ProjectFileName string
	ModuleName      string
	Githash         string
	PatchContent    string
	BuildVariants   []string
}

// PatchMetadata stores relevant patch information that is not
// strictly part of the patch data.
type PatchMetadata struct {
	Githash       string
	Project       *model.Project
	Module        *model.Module
	BuildVariants []string
	Summaries     []thirdparty.Summary
}

// Validate checks an API request to see if it is safe and sane.
// Returns the relevant patch metadata and any errors that occur.
func (pr *PatchAPIRequest) Validate(oauthToken string) (*PatchMetadata, error) {
	var repoOwner, repo string
	var module *model.Module
	projectRef, err := model.FindOneProjectRef(pr.ProjectFileName)
	if err != nil {
		return nil, fmt.Errorf("Could not find project ref %v : %v", pr.ProjectFileName, err)
	}

	repoOwner = projectRef.Owner
	repo = projectRef.Repo

	// validate the project file
	project, err := model.FindProject("", projectRef)
	if err != nil {
		return nil, fmt.Errorf("Could not find project file %v: %v",
			pr.ProjectFileName, err)
	}
	if project == nil {
		return nil, fmt.Errorf("No such project file named %v", pr.ProjectFileName)
	}

	if pr.ModuleName != "" {
		// is there a module? validate it.
		module, err = project.GetModuleByName(pr.ModuleName)
		if err != nil {
			return nil, fmt.Errorf("could not find module %v: %v", pr.ModuleName, err)
		}
		if module == nil {
			return nil, fmt.Errorf("no module named %v", pr.ModuleName)
		}
		repoOwner, repo = module.GetRepoOwnerAndName()
	}

	if len(pr.Githash) != 40 {
		return nil, fmt.Errorf("invalid githash")
	}
	gitCommit, err := thirdparty.GetCommitEvent(oauthToken, repoOwner, repo, pr.Githash)
	if err != nil {
		return nil, fmt.Errorf("could not find base revision %v for project %v: %v",
			pr.Githash, projectRef.Identifier, err)
	}
	if gitCommit == nil {
		return nil, fmt.Errorf("commit hash %v doesn't seem to exist", pr.Githash)
	}

	gitOutput, err := thirdparty.GitApplyNumstat(pr.PatchContent)
	if err != nil {
		return nil, fmt.Errorf("couldn't validate patch: %v", err)
	}
	if gitOutput == nil {
		return nil, fmt.Errorf("couldn't validate patch: git apply --numstat returned empty")
	}

	summaries, err := thirdparty.ParseGitSummary(gitOutput)
	if err != nil {
		return nil, fmt.Errorf("couldn't validate patch: %v", err)
	}

	if len(pr.BuildVariants) == 0 || pr.BuildVariants[0] == "" {
		return nil, fmt.Errorf("no buildvariants specified")
	}

	// verify that this build variant exists
	for _, buildVariant := range pr.BuildVariants {
		if buildVariant == "all" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return nil, fmt.Errorf("No such buildvariant: %v", buildVariant)
		}
	}
	return &PatchMetadata{pr.Githash, project, module, pr.BuildVariants, summaries}, nil
}

// Get the patch with the specified request it
func getPatchFromRequest(r *http.Request) (*patch.Patch, error) {
	// get id and secret from the request.
	vars := mux.Vars(r)
	patchIdStr := vars["patchId"]
	if len(patchIdStr) == 0 {
		return nil, fmt.Errorf("no patch id supplied")
	}
	if !patch.IsValidId(patchIdStr) {
		return nil, fmt.Errorf("patch id '%v' is not valid object id", patchIdStr)
	}

	// find the patch
	existingPatch, err := patch.FindOne(patch.ById(patch.NewId(patchIdStr)))
	if err != nil {
		return nil, err
	}
	if existingPatch == nil {
		return nil, fmt.Errorf("no existing request with id: %v", patchIdStr)
	}

	return existingPatch, nil
}

func (as *APIServer) submitPatch(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	var apiRequest PatchAPIRequest
	var projId, description string
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		apiRequest = PatchAPIRequest{
			ProjectFileName: r.FormValue("project"),
			ModuleName:      r.FormValue("module"),
			Githash:         r.FormValue("githash"),
			PatchContent:    r.FormValue("patch"),
			BuildVariants:   strings.Split(r.FormValue("buildvariants"), ","),
		}
		projId = r.FormValue("project")
		description = r.FormValue("desc")
	} else {
		data := struct {
			Description string `json:"desc"`
			Project     string `json:"project"`
			Patch       string `json:"patch"`
			Githash     string `json:"githash"`
			Variants    string `json:"buildvariants"`
			Finalize    bool   `json:"finalize"`
		}{}
		if err := util.ReadJSONInto(r.Body, &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if len(data.Patch) > patch.SizeLimit {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Patch is too large."))
		}
		description = data.Description
		projId = data.Project

		apiRequest = PatchAPIRequest{
			ProjectFileName: data.Project,
			ModuleName:      r.FormValue("module"),
			Githash:         data.Githash,
			PatchContent:    data.Patch,
			BuildVariants:   strings.Split(data.Variants, ","),
		}
	}

	projectRef, err := model.FindOneProjectRef(projId)
	if err != nil {
		message := fmt.Errorf("Error locating project ref '%v': %v",
			projId, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	project, err := model.FindProject("", projectRef)
	if err != nil {
		message := fmt.Errorf("Error locating project '%v' from '%v': %v",
			projId, as.Settings.ConfigDir, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if project == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("project %v not found", projId))
		return
	}

	patchMetadata, err := apiRequest.Validate(as.Settings.Credentials["github"])
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Invalid patch: %v", err))
		return
	}

	if patchMetadata == nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("patch metadata is empty"))
		return
	}

	if apiRequest.ModuleName != "" {
		as.WriteJSON(w, http.StatusBadRequest,
			"module not allowed when creating new patches (must be added in a subsequent request)")
		return
	}
	patchProjectRef, err := model.FindOneProjectRef(patchMetadata.Project.Identifier)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Invalid projectRef: %v", err))
		return
	}
	if patchProjectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("Empty patch project Ref"))
		return
	}

	commitInfo, err := thirdparty.GetCommitEvent(as.Settings.Credentials["github"],
		patchProjectRef.Owner,
		patchProjectRef.Repo,
		apiRequest.Githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if commitInfo == nil {
		as.WriteJSON(w, http.StatusBadRequest, "That commit doesn't seem to exist.")
		return
	}

	createTime := time.Now()

	// create a new object ID to use as reference for the patch data
	patchFileId := bson.NewObjectId().Hex()

	patchDoc := &patch.Patch{
		Id:            bson.NewObjectId(),
		Description:   description,
		Author:        user.Id,
		Project:       apiRequest.ProjectFileName,
		Githash:       apiRequest.Githash,
		CreateTime:    createTime,
		Status:        evergreen.PatchCreated,
		BuildVariants: apiRequest.BuildVariants,
		Tasks:         nil,
		Patches: []patch.ModulePatch{
			patch.ModulePatch{
				ModuleName: "",
				Githash:    apiRequest.Githash,
				PatchSet: patch.PatchSet{
					PatchFileId: patchFileId,
					Summary:     patchMetadata.Summaries,
				},
			},
		},
	}

	// write the patch content into a GridFS file under a new ObjectId.
	err = db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(apiRequest.PatchContent))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to write patch file to db: %v", err))
		return
	}

	// Get and validate patched config and add it to the patch document
	patchedProject, err := validator.GetPatchedProject(patchDoc, &as.Settings)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("invalid patched config: %v", err))
		return
	}

	projectYamlBytes, err := yaml.Marshal(patchedProject)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error marshalling patched config: %v", err))
		return
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = user.IncPatchNumber()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error computing patch num %v", err))
		return
	}
	patchDoc.PatchedConfig = string(projectYamlBytes)
	patchDoc.ClearPatchData()

	if err = patchDoc.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error inserting patch: %v", err))
		return
	}

	if strings.ToLower(r.FormValue("finalize")) == "true" {
		if _, err = model.FinalizePatch(patchDoc, &as.Settings); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	as.WriteJSON(w, http.StatusCreated, PatchAPIResponse{Patch: patchDoc})
}

func (as *APIServer) updatePatchModule(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	var moduleName, patchContent, githash string

	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		moduleName, patchContent, githash = r.FormValue("module"), r.FormValue("patch"), r.FormValue("githash")
	} else {
		data := struct {
			Module  string `json:"module"`
			Patch   string `json:"patch"`
			Githash string `json:"githash"`
		}{}
		if err := util.ReadJSONInto(r.Body, &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		moduleName, patchContent, githash = data.Module, data.Patch, data.Githash
	}

	projectRef, err := model.FindOneProjectRef(p.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error getting project ref with id %v: %v", p.Project, err))
		return
	}
	project, err := model.FindProject("", projectRef)
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
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("No such module", moduleName))
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

	commitInfo, err := thirdparty.GetCommitEvent(as.Settings.Credentials["github"], repoOwner, repo, githash)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if commitInfo == nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Errorf("commit hash doesn't seem to exist"))
		return
	}

	// write the patch content into a GridFS file under a new ObjectId.
	patchFileId := bson.NewObjectId().Hex()
	err = db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(patchContent))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("failed to write patch file to db: %v", err))
		return
	}

	modulePatch := patch.ModulePatch{
		ModuleName: moduleName,
		Githash:    githash,
		PatchSet: patch.PatchSet{
			PatchFileId: patchFileId,
			Summary:     summaries,
		},
	}

	if err = p.UpdateModulePatch(modulePatch); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, "Patch module updated")
	return
}

func (as *APIServer) listPatches(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	patches, err := patch.Find(patch.ByUser(user.Id))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			fmt.Errorf("error finding patches for user %v: %v", user.Id, err))
		return
	}
	as.WriteJSON(w, http.StatusOK, patches)
}

func (as *APIServer) existingPatchRequest(w http.ResponseWriter, r *http.Request) {
	p, err := getPatchFromRequest(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	if !getGlobalLock(r.RemoteAddr, p.Id.String()) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(r.RemoteAddr, p.Id.String())

	var action, desc string
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		action = r.FormValue("action")
	} else {
		data := struct {
			PatchId     string `json:"patch_id"`
			Action      string `json:"action"`
			Description string `json:"description"`
		}{}
		if err := util.ReadJSONInto(r.Body, &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		action, desc = data.Action, data.Description
	}

	// dispatch to handlers based on specified action
	switch action {
	case "update":
		err := p.SetDescription(desc)
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
		patchedProject, err := validator.GetPatchedProject(p, &as.Settings)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		projectYamlBytes, err := yaml.Marshal(patchedProject)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error marshalling patched config: %v", err))
			return
		}
		p.PatchedConfig = string(projectYamlBytes)
		_, err = model.FinalizePatch(p, &as.Settings)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		as.WriteJSON(w, http.StatusOK, "patch finalized")
	case "cancel":
		err = model.CancelPatch(p)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		as.WriteJSON(w, http.StatusOK, "patch deleted")
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
	as.WriteJSON(w, http.StatusOK, PatchAPIResponse{Patch: p})
}

func (as *APIServer) deletePatchModule(w http.ResponseWriter, r *http.Request) {
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

	as.WriteJSON(w, http.StatusOK, PatchAPIResponse{Message: "module removed from patch."})
}
