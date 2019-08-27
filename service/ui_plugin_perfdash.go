package service

import (
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/gimlet"
)

func perfDashGetTasksForVersion(w http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	projectId := vars["project_id"]
	versionId := vars["version_id"]

	if projectId == "" {
		http.Error(w, "empty project id", http.StatusBadRequest)
		return
	}
	if versionId == "" {
		http.Error(w, "empty version id", http.StatusBadRequest)
		return
	}
	projectRef, err := model.FindOneProjectRef(projectId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
	if projectRef == nil {
		http.Error(w, "empty project ref", http.StatusNotFound)
		return
	}
	v, err := model.VersionFindOne(model.VersionById(versionId).WithFields(model.VersionRevisionKey))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
	if v == nil {
		http.Error(w, "empty version", http.StatusNotFound)
		return
	}

	project, err := model.FindProjectFromVersionID(v.Revision)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return

	}
	if project == nil {
		http.Error(w, "empty project", http.StatusNotFound)
		return
	}

	if len(project.Tasks) == 0 {
		http.Error(w, fmt.Sprintf("no project tasks for project %v with revision %v", projectRef.Identifier, v.Revision),
			http.StatusBadRequest)
		return

	}

	taskMap := getVariantsWithCommand("json.send", project)
	gimlet.WriteJSON(w, taskMap)
}

// hasCommand returns true if the command name exists in the command
// or in the functions that may be in the command.
func hasCommand(commandName string, command model.PluginCommandConf, project *model.Project) bool {
	exists := false
	if command.Function != "" {
		for _, c := range project.Functions[command.Function].List() {
			exists = exists || hasCommand(commandName, c, project)
		}
	} else {
		exists = (command.Command == commandName && command.Params["name"] == "dashboard")
	}
	return exists
}

// createTaskCacheForCommand returns a map of tasks that have the command
func createTaskCacheForCommand(commandName string, project *model.Project) map[string]struct{} {
	tasks := map[string]struct{}{}
	for _, t := range project.Tasks {
		for _, command := range t.Commands {
			if hasCommand(commandName, command, project) {
				tasks[t.Name] = struct{}{}
				break
			}
		}
	}
	return tasks
}

// getVariantsWithCommand creates a cache of all tasks that have a command name
// and then iterates over all build variants to check if the task is in the cache,
// adds the bv name to a map which is returned as a mapping of the task name to the build variants.
func getVariantsWithCommand(commandName string, project *model.Project) map[string][]string {
	taskCache := createTaskCacheForCommand(commandName, project)
	buildVariants := map[string][]string{}
	for _, bv := range project.BuildVariants {
		for _, t := range bv.Tasks {
			if _, ok := taskCache[t.Name]; ok {
				variants, ok := buildVariants[t.Name]
				if !ok {
					buildVariants[t.Name] = []string{bv.Name}
				} else {
					buildVariants[t.Name] = append(variants, bv.Name)
				}
			}
		}
	}
	return buildVariants
}
