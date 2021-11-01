package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/shrub"
	"github.com/google/shlex"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	lintPrefix        = "lint"
	lintVariant       = "lint"
	lintGroup         = "lint-group"
	commitMaxHosts    = 4
	patchMaxHosts     = 1
	evergreenLintTask = "evergreen"
	jsonFilename      = "bin/generate-lint.json"
	scriptsDir        = "scripts"
	packagePrefix     = "github.com/evergreen-ci/evergreen"
)

// whatChanged returns a list of files that have changed in the working
// directory. First, it tries diffing changed files against the merge base. If
// there are no changes, this is not a patch build, so it diffs HEAD against HEAD~.
func whatChanged() ([]string, error) {
	mergeBaseCmd := exec.Command("git", "merge-base", "main@{upstream}", "HEAD")
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

			// We can't run make targets on packages in the cmd directory
			// because the packages contain dashes.
			if strings.HasPrefix(dir, "vendor") || strings.HasPrefix(dir, "cmd") {
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

func makeTarget(target string) string {
	return fmt.Sprintf("%s-%s", lintPrefix, target)
}

func getAllTargets() ([]string, error) {
	var targets []string

	gobin := "go"
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		gobin = filepath.Join(goroot, "bin", "go")
	}
	args, _ := shlex.Split(fmt.Sprintf("%s list -f '{{ join .Deps  \"\\n\"}}' cmd/evergreen/evergreen.go", gobin))
	cmd := exec.Command(args[0], args[1:]...)
	allPackages, err := cmd.Output()
	if err != nil {
		return nil, errors.Wrap(err, "problem getting diff")
	}
	split := strings.Split(strings.TrimSpace(string(allPackages)), "\n")
	for _, p := range split {
		if strings.HasPrefix(p, fmt.Sprintf("%s/vendor", packagePrefix)) {
			continue
		}

		if !strings.HasPrefix(p, packagePrefix) {
			continue
		}

		if p == packagePrefix {
			targets = append(targets, "evergreen")
			continue
		}
		p = strings.TrimPrefix(p, packagePrefix)
		p = strings.TrimPrefix(p, "/")
		p = strings.Replace(p, "/", "-", -1)
		targets = append(targets, p)
	}

	return targets, nil
}

// generateTasks returns a map of tasks to generate.
func generateTasks() (*shrub.Configuration, error) {
	changes, err := whatChanged()
	if err != nil {
		return nil, err
	}
	var targets []string
	var maxHosts int
	if len(changes) == 0 {
		maxHosts = commitMaxHosts
		targets, err = getAllTargets()
		if err != nil {
			return nil, err
		}
	} else {
		maxHosts = patchMaxHosts
		targets, err = targetsFromChangedFiles(changes)
		if err != nil {
			return nil, err
		}
	}

	conf := &shrub.Configuration{}
	if len(targets) == 0 {
		return conf, nil
	}

	lintTargets := []string{}
	for _, t := range targets {
		name := makeTarget(t)
		conf.Task(name).MustHaveTestResults(true).FunctionWithVars("run-make", map[string]string{"target": name})
		lintTargets = append(lintTargets, name)
	}

	group := conf.TaskGroup(lintGroup).SetMaxHosts(maxHosts)
	group.SetupGroup.Command().Type("system").Command("git.get_project").Param("directory", "gopath/src/github.com/evergreen-ci/evergreen")
	group.SetupGroup.Command().Function("setup-credentials")
	cmd := group.SetupGroup.Command().Function("run-make")
	cmd.Vars = map[string]string{"target": "get-go-imports"}
	group.TeardownTask.Command().Function("attach-test-results")
	group.TeardownTask.Command().Function("remove-test-results")
	group.Task(lintTargets...)

	conf.Variant(lintVariant).AddTasks(lintGroup)

	return conf, nil
}

func main() {
	generate, err := generateTasks()
	if err != nil {
		grip.EmergencyFatal(err)
	}
	jsonBytes, _ := json.MarshalIndent(generate, "", "  ")
	grip.Error(ioutil.WriteFile(jsonFilename, jsonBytes, 0644))
}
