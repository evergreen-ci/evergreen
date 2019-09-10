package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/evergreen-ci/shrub"
)

var (
	fileName = "bin/go-test-config.json"

	targets = []string{"test-agent", "test-auth", "test-cloud",
		"test-command", "test-db", "test-evergreen", "test-migrations", "test-model", "test-model-alertrecord", "test-model-artifact",
		"test-model-build", "test-model-distro", "test-model-event", "test-model-host", "test-model-notification", "test-model-patch",
		"test-model-stats", "test-model-task", "test-model-testresult", "test-model-user", "test-model-version", "test-monitor",
		"test-operations", "test-plugin", "test-repotracker", "test-rest-client", "test-rest-data", "test-rest-model", "test-rest-route",
		"test-scheduler", "test-service", "test-subprocess", "test-thirdparty", "test-trigger", "test-units", "test-util", "test-validator",
	}
)

func main() {
	config := makeTasks()
	jsonBytes, _ := json.MarshalIndent(config, "", "  ")
	ioutil.WriteFile(fileName, jsonBytes, 0644)
}

func makeTasks() *shrub.Configuration {
	config := &shrub.Configuration{}
	config.CommandType = "test"
	for _, target := range targets {
		config.Task(target).Function("get-project").
			Function("set-up-credentials").
			Function("set-up-mongodb").
			FunctionWithVars("run-make", map[string]string{"target": "revendor"}).
			FunctionWithVars("run-make", map[string]string{"target": target}).
			Function("attach-test-results")
	}
	ubuntu1604 := config.Variant("ubuntu1604")
	ubuntu1604.DisplayName("Ubuntu 16.04")
	ubuntu1604.RunOn("ubuntu1604-test")
	ubuntu1604.SetExpansions(map[string]interface{}{
		"gobin":            "/opt/golang/go1.9/bin/go",
		"disable_coverage": "yes",
		"goroot":           "/opt/golang/go1.9",
		"mongodb_url":      "https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1604-4.0.3.tgz",
	})
	ubuntu1604.AddTasks(targets...)

	config.Function("get-project").Append(&shrub.CommandDefinition{
		CommandName:   "git.get_project",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"directory": "gopath/src/github.com/evergreen-ci/evergreen",
			"token":     "${github_token}",
		},
	})
	config.Function("set-up-credentials").Append(&shrub.CommandDefinition{
		CommandName:   "subprocess.exec",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"silent":      true,
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen",
			"command":     "bash scripts/setup-credentials.sh",
			"env": map[string]string{
				"GITHUB_TOKEN": "${github_token}",
				"JIRA_SERVER":  "${jiraserver}",
				"CROWD_SERVER": "${crowdserver}",
				"CROWD_USER":   "${crowduser}",
				"CROWD_PW":     "${crowdpw}",
				"AWS_KEY":      "${aws_key}",
				"AWS_SECRET":   "${aws_secret}",
			},
		},
	})
	config.Function("set-up-mongodb").Append(&shrub.CommandDefinition{
		CommandName:   "subprocess.exec",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen/",
			"command":     "make get-mongodb",
			"env": map[string]string{
				"MONGODB_URL": "${mongodb_url}",
				"DECOMPRESS":  "${decompress}",
			},
		},
	}).Append(&shrub.CommandDefinition{
		CommandName:   "subprocess.exec",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen/",
			"background":  true,
			"command":     "make start-mongod",
		},
	}).Append(&shrub.CommandDefinition{
		CommandName:   "subprocess.exec",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen",
			"command":     "make check-mongod",
		},
	})
	config.Function("run-make").Append(&shrub.CommandDefinition{
		CommandName: "subprocess.exec",
		Params: map[string]interface{}{
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen",
			"binary":      "make",
			"args":        []string{"${make_args|}", "${target}"},
			"env": map[string]string{
				"AWS_KEY":           "${aws_key}",
				"AWS_SECRET":        "${aws_secret}",
				"DEBUG_ENABLED":     "${debug}",
				"DISABLE_COVERAGE":  "${disable_coverage}",
				"EVERGREEN_ALL":     "true",
				"GOARCH":            "${goarch}",
				"GO_BIN_PATH":       "${gobin}",
				"GOOS":              "${goos}",
				"GOPATH":            "${workdir}/gopath",
				"GOROOT":            "${goroot}",
				"RACE_DETECTOR":     "${race_detector}",
				"SETTINGS_OVERRIDE": "creds.yml",
				"TEST_TIMEOUT":      "${test_timeout}",
				"VENDOR_PKG":        "github.com/${trigger_repo_owner}/${trigger_repo_name}",
				"VENDOR_REVISION":   "${trigger_revision}",
				"XC_BUILD":          "${xc_build}",
			},
		},
	})
	config.Function("attach-test-results").Append(&shrub.CommandDefinition{
		CommandName:   "gotest.parse_files",
		ExecutionType: "system",
		Params: map[string]interface{}{
			"files": []string{"gopath/src/github.com/evergreen-ci/evergreen/bin/output.*"},
		},
	})
	return config
}
