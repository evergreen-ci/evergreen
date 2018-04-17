package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"strings"

	"github.com/google/shlex"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	lintPrefix        = "lint"
	lintVariant       = "ubuntu1604"
	lintGroup         = "lint-group"
	groupMaxHosts     = 1
	evergreenLintTask = "evergreen"
	jsonFilename      = "bin/generate-lint.json"
	scriptsDir        = "scripts"
	packagePrefix     = "github.com/evergreen-ci/evergreen"
)

// whatChanged returns a list of files that have changed in the working
// directory. First, it tries diffing changed files against the merge base. If
// there are no changes, this is not a patch build, so it diffs HEAD against HEAD~.
func whatChanged() ([]string, error) {
	mergeBaseCmd := exec.Command("git", "merge-base", "master@{upstream}", "HEAD")
	base, err := mergeBaseCmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting merge-base")
	}
	diffCmd := exec.Command("git", "diff", strings.TrimSpace(string(base)), "--name-only")
	files, err := diffCmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting diff")
	}
	var split []string
	// if there is no diff, this is not a patch build
	if len(files) == 0 {
		return []string{}, nil
	}
	split = strings.Split(strings.TrimSpace(string(files)), "\n")
	return split, nil
}

// targetsFromChangedFiles returns a list of make targets.
func targetsFromChangedFiles(files []string) ([]string, error) {
	targets := map[string]struct{}{}
	for _, f := range files {
		filePath := strings.TrimSpace(f)
		if strings.HasSuffix(filePath, ".go") {
			dir := path.Dir(filePath)
			if dir == scriptsDir {
				continue
			}
			if dir == "." || dir == "main" {
				targets["evergreen"] = struct{}{}
			} else {
				targets[strings.Replace(dir, "/", "-", -1)] = struct{}{}
			}
		}
	}
	targetSlice := []string{}
	for t := range targets {
		targetSlice = append(targetSlice, t)
	}
	return targetSlice, nil
}

// makeTask returns a task map that can be marshaled into a JSON document.
func makeTask(target string) map[string]interface{} {
	name := makeTarget(target)
	task := map[string]interface{}{
		"name": name,
		"commands": []map[string]interface{}{
			map[string]interface{}{
				"func": "run-make",
				"vars": map[string]interface{}{
					"target": name,
				},
			},
		},
	}
	return task
}

func makeTarget(target string) string {
	return fmt.Sprintf("%s-%s", lintPrefix, target)
}

// generateTasks returns a map of tasks to generate.
func generateTasks() (map[string][]map[string]interface{}, error) {
	changes, err := whatChanged()
	if err != nil {
		return nil, err
	}
	var targets []string
	if len(changes) == 0 {
		args, _ := shlex.Split("go list -f '{{ join .Deps  \"\\n\"}}' main/evergreen.go")
		cmd := exec.Command(args[0], args[1:]...)
		allPackages, err := cmd.Output()
		if err != nil {
			return nil, errors.Wrap(err, "problem getting diff")
		}
		split := strings.Split(strings.TrimSpace(string(allPackages)), "\n")
		for _, p := range split {
			if !strings.Contains(p, "vendor") && strings.Contains(p, "evergreen") {
				if p == packagePrefix {
					continue
				}
				p = strings.TrimPrefix(p, packagePrefix)
				p = strings.TrimPrefix(p, "/")
				p = strings.Replace(p, "/", "-", -1)
				targets = append(targets, p)
			}
		}
	} else {
		targets, err = targetsFromChangedFiles(changes)
		if err != nil {
			return nil, err
		}
	}
	taskList := []map[string]interface{}{}
	bvTasksList := []map[string]string{}
	for _, t := range targets {
		taskList = append(taskList, makeTask(t))
		bvTasksList = append(bvTasksList, map[string]string{"name": makeTarget(t)})
	}
	executionTaskList := []string{}
	for _, et := range taskList {
		executionTaskList = append(executionTaskList, et["name"].(string))
	}
	if len(executionTaskList) == 0 {
		return nil, nil
	}
	generate := map[string][]map[string]interface{}{}
	generate["tasks"] = taskList
	generate["task_groups"] = []map[string]interface{}{
		map[string]interface{}{
			"name":      lintGroup,
			"max_hosts": groupMaxHosts,
			"tasks":     executionTaskList,
			"setup_group": []map[string]interface{}{
				map[string]interface{}{
					"command": "git.get_project",
					"type":    "system",
					"params": map[string]string{
						"directory": "gopath/src/github.com/evergreen-ci/evergreen",
					},
				},
				map[string]interface{}{
					"func": "set-up-credentials",
				},
			},
			"teardown_task": []map[string]interface{}{
				{"func": "attach-test-results"},
				{"func": "remove-test-results"},
			},
		},
	}
	generate["buildvariants"] = []map[string]interface{}{
		map[string]interface{}{
			"name":  lintVariant,
			"tasks": []string{lintGroup},
			"display_tasks": []map[string]interface{}{
				map[string]interface{}{
					"name":            lintPrefix,
					"execution_tasks": executionTaskList,
				},
			},
		},
	}
	return generate, nil
}

func main() {
	generate, err := generateTasks()
	if err != nil {
		grip.EmergencyFatal(err)
	}
	jsonBytes, _ := json.MarshalIndent(generate, "", "  ")
	ioutil.WriteFile(jsonFilename, jsonBytes, 0644)
}
