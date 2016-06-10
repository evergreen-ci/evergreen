package service

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/gorilla/mux"
	"gopkg.in/mgo.v2/bson"
	"gopkg.in/yaml.v2"
)

// PatchAPIResponse is returned by all patch-related API calls
type PatchAPIResponse struct {
	Message string       `json:"message"`
	Action  string       `json:"action"`
	Patch   *patch.Patch `json:"patch"`
}

// PatchAPIRequest in the input struct with which we process patch requests
type PatchAPIRequest struct {
	ProjectId     string
	ModuleName    string
	Githash       string
	PatchContent  string
	BuildVariants []string
	Tasks         []string
	Description   string
}

// CreatePatch checks an API request to see if it is safe and sane.
// Returns the relevant patch metadata, the patch document, and any errors that occur.
func (pr *PatchAPIRequest) CreatePatch(finalize bool, oauthToken string,
	dbUser *user.DBUser, settings *evergreen.Settings) (*model.Project, *patch.Patch, error) {
	var repoOwner, repo string
	var module *model.Module

	projectRef, err := model.FindOneProjectRef(pr.ProjectId)
	if err != nil {
		return nil, nil, fmt.Errorf("Could not find project ref %v : %v", pr.ProjectId, err)
	}

	repoOwner = projectRef.Owner
	repo = projectRef.Repo

	if len(pr.Githash) != 40 {
		return nil, nil, fmt.Errorf("invalid githash")
	}

	gitCommit, err := thirdparty.GetCommitEvent(oauthToken, repoOwner, repo, pr.Githash)
	if err != nil {
		return nil, nil, fmt.Errorf("could not find base revision %v for project %v: %v",
			pr.Githash, projectRef.Identifier, err)

	}
	if gitCommit == nil {
		return nil, nil, fmt.Errorf("commit hash %v doesn't seem to exist", pr.Githash)
	}

	gitOutput, err := thirdparty.GitApplyNumstat(pr.PatchContent)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't validate patch: %v", err)
	}
	if gitOutput == nil {
		return nil, nil, fmt.Errorf("couldn't validate patch: git apply --numstat returned empty")
	}

	summaries, err := thirdparty.ParseGitSummary(gitOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("couldn't validate patch: %v", err)
	}

	if finalize && (len(pr.BuildVariants) == 0 || pr.BuildVariants[0] == "") {
		return nil, nil, fmt.Errorf("no buildvariants specified")
	}

	createTime := time.Now()

	// create a new object ID to use as reference for the patch data
	patchFileId := bson.NewObjectId().Hex()
	patchDoc := &patch.Patch{
		Id:            bson.NewObjectId(),
		Description:   pr.Description,
		Author:        dbUser.Id,
		Project:       pr.ProjectId,
		Githash:       pr.Githash,
		CreateTime:    createTime,
		Status:        evergreen.PatchCreated,
		BuildVariants: pr.BuildVariants,
		Tasks:         pr.Tasks,
		Patches: []patch.ModulePatch{
			{
				ModuleName: "",
				Githash:    pr.Githash,
				PatchSet: patch.PatchSet{
					Patch:       pr.PatchContent,
					PatchFileId: patchFileId,
					Summary:     summaries,
				},
			},
		},
	}

	// Get and validate patched config and add it to the patch document
	project, err := validator.GetPatchedProject(patchDoc, settings)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid patched config: %v", err)
	}

	if pr.ModuleName != "" {
		// is there a module? validate it.
		module, err = project.GetModuleByName(pr.ModuleName)
		if err != nil {
			return nil, nil, fmt.Errorf("could not find module %v: %v", pr.ModuleName, err)
		}
		if module == nil {
			return nil, nil, fmt.Errorf("no module named %v", pr.ModuleName)
		}
	}

	// verify that all variants exists
	for _, buildVariant := range pr.BuildVariants {
		if buildVariant == "all" || buildVariant == "" {
			continue
		}
		bv := project.FindBuildVariant(buildVariant)
		if bv == nil {
			return nil, nil, fmt.Errorf("No such buildvariant: %v", buildVariant)
		}
	}

	// write the patch content into a GridFS file under a new ObjectId after validating.
	err = db.WriteGridFile(patch.GridFSPrefix, patchFileId, strings.NewReader(pr.PatchContent))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to write patch file to db: %v", err)
	}

	// add the project config
	projectYamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return nil, nil, fmt.Errorf("error marshalling patched config: %v", err)
	}

	// set the patch number based on patch author
	patchDoc.PatchNumber, err = dbUser.IncPatchNumber()
	if err != nil {
		return nil, nil, fmt.Errorf("error computing patch num %v", err)
	}
	patchDoc.PatchedConfig = string(projectYamlBytes)

	patchDoc.ClearPatchData()

	return project, patchDoc, nil
}

// submitPatch creates the Patch document, adds the patched project config to it,
// and saves the patches to GridFS to be retrieved
func (as *APIServer) submitPatch(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)
	var apiRequest PatchAPIRequest
	var finalize bool
	if r.Header.Get("Content-Type") == "application/x-www-form-urlencoded" {
		patchContent := r.FormValue("patch")
		if patchContent == "" {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Error: Patch must not be empty"))
			return
		}
		apiRequest = PatchAPIRequest{
			ProjectId:     r.FormValue("project"),
			ModuleName:    r.FormValue("module"),
			Githash:       r.FormValue("githash"),
			PatchContent:  r.FormValue("patch"),
			BuildVariants: strings.Split(r.FormValue("buildvariants"), ","),
			Description:   r.FormValue("desc"),
		}
		finalize = strings.ToLower(r.FormValue("finalize")) == "true"
	} else {
		data := struct {
			Description string   `json:"desc"`
			Project     string   `json:"project"`
			Patch       string   `json:"patch"`
			Githash     string   `json:"githash"`
			Variants    string   `json:"buildvariants"`
			Tasks       []string `json:"tasks"`
			Finalize    bool     `json:"finalize"`
		}{}
		if err := util.ReadJSONInto(r.Body, &data); err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if len(data.Patch) > patch.SizeLimit {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Patch is too large."))
		}
		if len(data.Patch) == 0 {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Error: Patch must not be empty"))
			return
		}
		finalize = data.Finalize

		apiRequest = PatchAPIRequest{
			ProjectId:     data.Project,
			ModuleName:    r.FormValue("module"),
			Githash:       data.Githash,
			PatchContent:  data.Patch,
			BuildVariants: strings.Split(data.Variants, ","),
			Tasks:         data.Tasks,
			Description:   data.Description,
		}
	}

	project, patchDoc, err := apiRequest.CreatePatch(
		finalize, as.Settings.Credentials["github"], dbUser, &as.Settings)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Invalid patch: %v", err))
		return
	}

	//expand tasks and build variants and include dependencies
	if len(patchDoc.BuildVariants) == 1 && patchDoc.BuildVariants[0] == "all" {
		patchDoc.BuildVariants = []string{}
		for _, buildVariant := range project.BuildVariants {
			if buildVariant.Disabled {
				continue
			}
			patchDoc.BuildVariants = append(patchDoc.BuildVariants, buildVariant.Name)
		}
	}

	if len(patchDoc.Tasks) == 1 && patchDoc.Tasks[0] == "all" {
		patchDoc.Tasks = []string{}
		for _, t := range project.Tasks {
			if t.Patchable != nil && !(*t.Patchable) {
				continue
			}
			patchDoc.Tasks = append(patchDoc.Tasks, t.Name)
		}
	}

	var pairs []model.TVPair
	for _, v := range patchDoc.BuildVariants {
		for _, t := range patchDoc.Tasks {
			if project.FindTaskForVariant(t, v) != nil {
				pairs = append(pairs, model.TVPair{v, t})
			}
		}
	}

	// update variant and tasks to include dependencies
	pairs = model.IncludePatchDependencies(project, pairs)

	patchDoc.SyncVariantsTasks(model.TVPairsToVariantTasks(pairs))

	if err = patchDoc.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("error inserting patch: %v", err))
		return
	}

	if finalize {
		if _, err = model.FinalizePatch(patchDoc, &as.Settings); err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	as.WriteJSON(w, http.StatusCreated, PatchAPIResponse{Patch: patchDoc})
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

	if len(patchContent) == 0 {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Error: Patch must not be empty"))
		return
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

// listPatches returns a user's "n" most recent patches.
func (as *APIServer) listPatches(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)
	n, err := util.GetIntValue(r, "n", 0)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("cannot read value n: %v", err))
		return
	}
	query := patch.ByUser(dbUser.Id).Sort([]string{"-" + patch.CreateTimeKey})
	if n > 0 {
		query = query.Limit(n)
	}
	patches, err := patch.Find(query)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError,
			fmt.Errorf("error finding patches for user %v: %v", dbUser.Id, err))
		return
	}
	as.WriteJSON(w, http.StatusOK, patches)
}

func (as *APIServer) existingPatchRequest(w http.ResponseWriter, r *http.Request) {
	dbUser := MustHaveUser(r)

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
		err = model.CancelPatch(p, dbUser.Id)
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
