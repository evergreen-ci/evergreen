package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/pkg/errors"
)

const (
	lintPrefix        = "lint"
	lintVariant       = "race-detector"
	evergreenLintTask = "evergreen"
	jsonFilename      = "generate-lint.json"
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
	// if there is no diff, this is not a patch build, so diff against HEAD~
	if len(files) == 0 {
		diffCmd := exec.Command("git", "diff", "HEAD~", "--name-only")
		files, err = diffCmd.Output()
		if err != nil {
			return nil, errors.Wrap(err, "problem getting diff")
		}
	}
	split = strings.Split(strings.TrimSpace(string(files)), "\n")
	return split, nil
}

// targetsFromChangedFiles returns a list of make targets.
func targetsFromChangedFiles(files []string) ([]string, error) {
	targets := map[string]interface{}{}
	for _, f := range files {
		filePath := strings.TrimSpace(f)
		if strings.HasSuffix(filePath, ".go") {
			dir := path.Dir(filePath)
			if dir == "scripts" {
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
				"func": "get-project",
			},
			map[string]interface{}{
				"func": "set-up-credentials",
			},
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
	targets, err := targetsFromChangedFiles(changes)
	if err != nil {
		return nil, err
	}
	taskList := []map[string]interface{}{}
	bvTasksList := []map[string]string{}
	for _, t := range targets {
		taskList = append(taskList, makeTask(t))
		bvTasksList = append(bvTasksList, map[string]string{"name": makeTarget(t)})
	}
	generate := map[string][]map[string]interface{}{}
	generate["tasks"] = taskList
	generate["buildvariants"] = []map[string]interface{}{
		map[string]interface{}{
			"name":  lintVariant,
			"tasks": bvTasksList,
		},
	}
	return generate, nil
}

func main() {
	generate, err := generateTasks()
	if err != nil {
		fmt.Printf("ERROR: %+v\n", err)
		os.Exit(1)
	}
	jsonBytes, _ := json.MarshalIndent(generate, "", "  ")
	ioutil.WriteFile(jsonFilename, jsonBytes, 0644)
}
