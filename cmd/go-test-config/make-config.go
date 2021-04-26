package main

import (
	"encoding/json"
	"io/ioutil"

	"github.com/evergreen-ci/shrub"
	"github.com/mongodb/grip"
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
	grip.Error(ioutil.WriteFile(fileName, jsonBytes, 0644))
}

func makeTasks() *shrub.Configuration {
	config := &shrub.Configuration{}
	config.CommandType = "test"
	for _, target := range targets {
		config.Task(target).Function("get-project").
			Function("setup-credentials").
			Function("setup-mongodb").
			FunctionWithVars("run-make", map[string]string{"target": "revendor"}).
			FunctionWithVars("run-make", map[string]string{"target": target}).
			Function("attach-test-results")
	}
	_ = config.Variant("ubuntu1604").AddTasks(targets...)

	config.Function("get-project").Append(&shrub.CommandDefinition{
		CommandName:   "git.get_project",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"directory": "gopath/src/github.com/evergreen-ci/evergreen",
			"token":     "${github_token}",
		},
	})
	config.Function("setup-credentials").Append(&shrub.CommandDefinition{
		CommandName:   "subprocess.exec",
		ExecutionType: "setup",
		Params: map[string]interface{}{
			"silent":      true,
			"working_dir": "gopath/src/github.com/evergreen-ci/evergreen",
			"command":     "bash scripts/setup-credentials.sh",
			"env": map[string]string{
				"GITHUB_TOKEN":      "${github_token}",
				"JIRA_SERVER":       "${jiraserver}",
				"CROWD_SERVER":      "${crowdserver}",
				"AWS_KEY":           "${aws_key}",
				"AWS_SECRET":        "${aws_secret}",
				"JIRA_PRIVATE_KEY":  "${jira_private_key}",
				"JIRA_ACCESS_TOKEN": "${jira_access_token}",
				"JIRA_TOKEN_SECRET": "${jira_token_secret}",
				"JIRA_CONSUMER_KEY": "${jira_consumer_key}",
			},
		},
	})
	config.Function("setup-mongodb").Append(&shrub.CommandDefinition{
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
