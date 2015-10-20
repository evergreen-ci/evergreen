package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"gopkg.in/yaml.v2"
	"net/http"
	"strconv"
)

var (
	TaskNotPatchableError = fmt.Errorf("Task is not patchable.")
)

func (uis *UIServer) patchPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	var versionAsUI *uiVersion
	if projCtx.Version != nil { // Patch is already finalized
		versionAsUI = &uiVersion{
			Version:   *projCtx.Version,
			RepoOwner: projCtx.ProjectRef.Owner,
			Repo:      projCtx.ProjectRef.Repo,
		}
	}

	// get the new patch document with the patched configuration
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshal the patch's project config so that it is always up to date with the configuration file in the project
	project := &model.Project{}
	if err := yaml.Unmarshal([]byte(projCtx.Patch.PatchedConfig), project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error unmarshaling project config: %v", err))
	}
	projCtx.Project = project

	// retrieve tasks and variant mappings' names
	variantMappings := make(map[string]model.BuildVariant)
	for _, variant := range projCtx.Project.BuildVariants {
		variantMappings[variant.Name] = variant
	}

	tasksList := []interface{}{}
	for _, task := range projCtx.Project.Tasks {
		// add a task name to the list if it's patchable
		if !(task.Patchable != nil && *task.Patchable == false) {
			tasksList = append(tasksList, struct{ Name string }{task.Name})
		}
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		ProjectData projectContext
		User        *user.DBUser
		Version     *uiVersion
		Variants    map[string]model.BuildVariant
		Tasks       []interface{}
	}{projCtx, GetUser(r), versionAsUI, variantMappings, tasksList}, "base",
		"patch_version.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) schedulePatch(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}

	// grab patch again, as the diff  was excluded
	var err error
	projCtx.Patch, err = patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err), http.StatusInternalServerError)
		return
	}

	// Unmarshal the project config and set it in the project context
	project := &model.Project{}
	if err := yaml.Unmarshal([]byte(projCtx.Patch.PatchedConfig), project); err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error unmarshaling project config: %v", err))
	}
	projCtx.Project = project

	patchUpdateReq := struct {
		Variants    []string `json:"variants"`
		Tasks       []string `json:"tasks"`
		Description string   `json:"description"`
	}{}

	err = util.ReadJSONInto(r.Body, &patchUpdateReq)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Add all dependencies to patchUpdateReq.Tasks and add their variants to patchUpdateReq.Variants

	// Construct a set of variants to include in patchUpdateReq.Variants
	updateReqVariants := make(map[string]bool)
	for _, variant := range patchUpdateReq.Variants {
		updateReqVariants[variant] = true
	}

	// Construct a set of tasks to include in patchUpdateReq.Tasks
	// Add all dependencies, and add their variants to updateReqVariants
	updateReqTasks := make(map[string]bool)
	for _, v := range patchUpdateReq.Variants {
		for _, t := range projCtx.Project.FindTasksForVariant(v) {
			for _, task := range patchUpdateReq.Tasks {
				if t == task {
					deps, variants, err := getDeps(task, v, projCtx.Project)
					if err != nil {
						if err == TaskNotPatchableError {
							continue
						} else {
							uis.LoggedError(w, r, http.StatusInternalServerError,
								fmt.Errorf("Error getting dependencies for task: %v", err))
							return
						}
					}
					updateReqTasks[task] = true
					for _, dep := range deps {
						updateReqTasks[dep] = true
					}
					for _, variant := range variants {
						updateReqVariants[variant] = true
					}
				}
			}
		}
	}

	// Reset patchUpdateReq.Tasks and patchUpdateReq.Variants
	patchUpdateReq.Tasks = make([]string, 0, len(updateReqTasks))
	for task := range updateReqTasks {
		patchUpdateReq.Tasks = append(patchUpdateReq.Tasks, task)
	}
	patchUpdateReq.Variants = make([]string, 0, len(updateReqVariants))
	for variant := range updateReqVariants {
		patchUpdateReq.Variants = append(patchUpdateReq.Variants, variant)
	}

	if projCtx.Patch.Version != "" {
		// This patch has already been finalized, just add the new builds and tasks
		if projCtx.Version == nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Couldn't find patch for id %v", projCtx.Patch.Version))
			return
		}
		// First add new tasks to existing builds, if necessary
		if len(patchUpdateReq.Tasks) > 0 {
			err = model.AddNewTasksForPatch(projCtx.Patch, projCtx.Version, projCtx.Project, patchUpdateReq.Tasks)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError,
					fmt.Errorf("Error creating new tasks: `%v` for version `%v`", err, projCtx.Version.Id))
				return
			}
		}

		if len(patchUpdateReq.Variants) > 0 {
			_, err := model.AddNewBuildsForPatch(projCtx.Patch, projCtx.Version, projCtx.Project, patchUpdateReq.Variants)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError,
					fmt.Errorf("Error creating new builds: `%v` for version `%v`", err, projCtx.Version.Id))
				return
			}
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Builds and tasks successfully added to patch."))
		uis.WriteJSON(w, http.StatusOK, struct {
			VersionId string `json:"version"`
		}{projCtx.Version.Id})
	} else {
		err = projCtx.Patch.SetVariantsAndTasks(patchUpdateReq.Variants, patchUpdateReq.Tasks)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Error setting patch variants and tasks: %v", err))
			return
		}

		if err = projCtx.Patch.SetDescription(patchUpdateReq.Description); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Error setting description: %v", err))
			return
		}

		ver, err := model.FinalizePatch(projCtx.Patch, &uis.Settings)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error finalizing patch: %v", err))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Patch builds are scheduled."))
		uis.WriteJSON(w, http.StatusOK, struct {
			VersionId string `json:"version"`
		}{ver.Id})
	}
}

// getDeps returns all recursive dependencies of task and their variants.
// If task has a non-patchable dependency, getDeps will return TaskNotPatchableError
// The returned slices may contain duplicates.
func getDeps(task string, variant string, p *model.Project) ([]string, []string, error) {
	projectTask := p.FindTaskForVariant(task, variant)
	if projectTask == nil {
		return nil, nil, fmt.Errorf("Task not found in project: %v", task)
	}
	if patchable := projectTask.Patchable; (patchable != nil && !*patchable) || task == evergreen.PushStage { //TODO remove PushStage
		return nil, nil, TaskNotPatchableError
	}
	deps := make([]string, 0)
	variants := make([]string, 0)
	for _, dep := range projectTask.DependsOn {
		if dep.Variant == model.AllVariants {
			if dep.Name == model.AllDependencies {
				// Case: name = *, variant = *
				for _, v := range p.FindAllVariants() {
					variants = append(variants, v)
					for _, t := range p.FindTasksForVariant(v) {
						if t == task && v == variant {
							continue
						}
						if depTask := p.FindTaskForVariant(t, v); t == evergreen.PushStage ||
							(depTask.Patchable != nil && !*depTask.Patchable) {
							return nil, nil, TaskNotPatchableError // TODO remove PushStage
						}
						deps = append(deps, t)
					}
				}
			} else {
				// Case: variant = *, specific name
				deps = append(deps, dep.Name)
				for _, v := range p.FindAllVariants() {
					for _, t := range p.FindTasksForVariant(v) {
						if t == dep.Name {
							if t == task && v == variant {
								continue
							}
							recDeps, recVariants, err := getDeps(t, v, p)
							if err != nil {
								return nil, nil, err
							}
							deps = append(deps, recDeps...)
							variants = append(variants, v)
							variants = append(variants, recVariants...)
						}
					}
				}
			}
		} else {
			if dep.Name == model.AllDependencies {
				// Case: name = *, specific variant
				v := dep.Variant
				if v == "" {
					v = variant
				}
				variants = append(variants, v)
				for _, t := range p.FindTasksForVariant(v) {
					if t == task && v == variant {
						continue
					}
					recDeps, recVariants, err := getDeps(t, v, p)
					if err != nil {
						return nil, nil, err
					}
					deps = append(deps, t)
					deps = append(deps, recDeps...)
					variants = append(variants, recVariants...)
				}
			} else {
				// Case: specific name, specific variant
				v := dep.Variant
				if v == "" {
					v = variant
				}
				recDeps, recVariants, err := getDeps(dep.Name, v, p)
				if err != nil {
					return nil, nil, err
				}
				deps = append(deps, dep.Name)
				deps = append(deps, recDeps...)
				variants = append(variants, v)
				variants = append(variants, recVariants...)
			}
		}
	}
	return deps, variants, nil
}

func (uis *UIServer) diffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	// We have to reload the patch outside of the project context,
	// since the raw diff is excluded by default. This redundancy is
	// worth the time savings this behavior offers other pages.
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	fullPatch.FetchPatchFiles()
	uis.WriteHTML(w, http.StatusOK, fullPatch, "base", "diff.html")
}

func (uis *UIServer) fileDiffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	fullPatch.FetchPatchFiles()
	uis.WriteHTML(w, http.StatusOK, struct {
		Data        patch.Patch
		FileName    string
		PatchNumber string
	}{*fullPatch, r.FormValue("file_name"), r.FormValue("patch_number")},
		"base", "file_diff.html")
}

func (uis *UIServer) rawDiffPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	fullPatch, err := patch.FindOne(patch.ById(projCtx.Patch.Id))
	if err != nil {
		http.Error(w, fmt.Sprintf("error loading patch: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	fullPatch.FetchPatchFiles()
	patchNum, err := strconv.Atoi(r.FormValue("patch_number"))
	if err != nil {
		http.Error(w, fmt.Sprintf("error getting patch number: %v", err.Error),
			http.StatusInternalServerError)
		return
	}
	if patchNum < 0 || patchNum >= len(fullPatch.Patches) {
		http.Error(w, "patch number out of range", http.StatusInternalServerError)
		return
	}
	diff := fullPatch.Patches[patchNum].PatchSet.Patch
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(diff))
}
