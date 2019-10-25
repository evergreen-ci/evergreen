package operations

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/google/go-github/github"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// uiPort is the local port the UI will listen on.
	smokeUiPort = ":9090"
	// urlPrefix is the localhost prefix for accessing local Evergreen.
	smokeUrlPrefix = "http://localhost"
)

// smokeEndpointTestDefinitions describes the UI and API endpoints to verify are up.
type smokeEndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

func (tests smokeEndpointTestDefinitions) checkEndpoints(username, key string) error {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)
	client.Timeout = time.Second

	// wait for web service to start
	attempts := 10
	for i := 1; i <= attempts; i++ {
		grip.Infof("checking if Evergreen is up (attempt %d of %d)", i, attempts)
		_, err := client.Get(smokeUrlPrefix + smokeUiPort)
		if err != nil {
			if i == attempts {
				err = errors.Wrapf(err, "could not connect to Evergreen after %d attempts", attempts)
				grip.Error(err)
				return err
			}
			grip.Infof("could not connect to Evergreen (attempt %d of %d)", i, attempts)
			time.Sleep(time.Second)
			continue
		}
	}
	grip.Info("Evergreen is up")

	// check endpoints
	catcher := grip.NewSimpleCatcher()
	grip.Info("Testing UI Endpoints")
	for url, expected := range tests.UI {
		catcher.Add(makeSmokeRequestAndCheck(username, key, client, url, expected))
	}

	grip.Info("Testing API Endpoints")
	for url, expected := range tests.API {
		catcher.Add(makeSmokeRequestAndCheck(username, key, client, "/api"+url, expected))
	}

	grip.InfoWhen(!catcher.HasErrors(), "success: all endpoints accessible")

	return errors.Wrapf(catcher.Resolve(), "failed to get %d endpoints", catcher.Len())
}

func getLatestGithubCommit() (string, error) {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	resp, err := client.Get("https://api.github.com/repos/evergreen-ci/evergreen/git/refs/heads/master")
	if err != nil {
		return "", errors.Wrap(err, "failed to get latest commit from GitHub")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "error reading response body from GitHub")
	}

	latest := github.Reference{}
	if err = json.Unmarshal(body, &latest); err != nil {
		return "", errors.Wrap(err, "error unmarshaling response from GitHub")
	}
	if latest.Object != nil && latest.Object.SHA != nil && *latest.Object.SHA != "" {
		return *latest.Object.SHA, nil
	}
	return "", errors.New("could not find latest commit in response")
}

func checkTaskByCommit(username, key string) error {
	client := util.GetHTTPClient()
	defer util.PutHTTPClient(client)

	var builds []apimodels.APIBuild
	var build apimodels.APIBuild
	for i := 0; i <= 30; i++ {
		// get task id
		if i == 30 {
			return errors.New("error getting builds for version")
		}
		time.Sleep(10 * time.Second)

		latest, err := getLatestGithubCommit()
		if err != nil {
			grip.Error(errors.Wrap(err, "error getting latest GitHub commit"))
			continue
		}

		grip.Infof("checking for a build of %s (%d/30)", latest, i+1)

		body, err := makeSmokeRequest(username, key, client, "/rest/v2/versions/evergreen_"+latest+"/builds")
		if err != nil {
			grip.Error(err)
			continue
		}

		err = json.Unmarshal(body, &builds)
		if err != nil {
			err = json.Unmarshal(body, &build)
			if err != nil {
				return errors.Wrap(err, "error unmarshaling json")
			}
		}

		if len(builds) == 0 {
			builds = []apimodels.APIBuild{build}
		}

		if len(builds[0].Tasks) == 0 {
			builds = []apimodels.APIBuild{}
			build = apimodels.APIBuild{}

			continue
		}
		break
	}

	var task apimodels.APITask
OUTER:
	for i := 0; i <= 30; i++ {
		// check task
		if i == 30 {
			return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
		}
		time.Sleep(10 * time.Second)
		grip.Infof("checking for %d tasks (%d/30)", len(builds[0].Tasks), i+1)

		var err error
		for t := 0; t < len(builds[0].Tasks); t++ {
			task, err = checkTask(client, username, key, builds, t)
			if err != nil {
				return errors.WithStack(err)
			}

			if task.Status == evergreen.TaskFailed {
				return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
			}
			if task.Status != evergreen.TaskSucceeded {
				grip.Infof("found task is status %s", task.Status)
				task = apimodels.APITask{}
				continue OUTER
			}

			grip.Infof("checking for log %s", task.Logs["task_log"])
			body, err := makeSmokeRequest(username, key, client, task.Logs["task_log"]+"&text=true")
			if err != nil {
				return errors.Wrap(err, "error getting log data")
			}
			page := string(body)

			// Validate that task contains task completed message
			if strings.Contains(page, "Task completed - SUCCESS") {
				grip.Infof("Found task completed message in log:\n%s", page)
			} else {
				grip.Errorf("did not find task completed message in log:\n%s", page)
				return errors.New("did not find task completed message in log")
			}

			// Validate that setup_group only runs in first task
			if strings.Contains(page, "first") {
				if !strings.Contains(page, "setup_group") {
					return errors.New("did not find setup_group in logs for first task")
				}
			} else {
				if strings.Contains(page, "setup_group") {
					return errors.New("setup_group should only run in first task")
				}
			}

			// Validate that setup_task and teardown_task run for all tasks
			if !strings.Contains(page, "setup_task") {
				return errors.New("did not find setup_task in logs")
			}
			if !strings.Contains(page, "teardown_task") {
				return errors.New("did not find teardown_task in logs")
			}

			// Validate that teardown_group only runs in last task
			if strings.Contains(page, "fourth") {
				if !strings.Contains(page, "teardown_group") {
					return errors.New("did not find teardown_group in logs for last (fourth) task")
				}
			} else {
				if strings.Contains(page, "teardown_group") {
					return errors.New("teardown_group should only run in last (fourth) task")
				}
			}
		}
		grip.Info("Successfully checked tasks")
		return nil
	}
	return errors.New("this code should be unreachable")
}

func checkTask(client *http.Client, username, key string, builds []apimodels.APIBuild, taskIndex int) (apimodels.APITask, error) {
	task := apimodels.APITask{}
	grip.Infof("checking for task %s", builds[0].Tasks[taskIndex])
	r, err := http.NewRequest("GET", smokeUrlPrefix+smokeUiPort+"/rest/v2/tasks/"+builds[0].Tasks[taskIndex], nil)
	if err != nil {
		return task, errors.Wrap(err, "failed to make request")
	}
	r.Header.Add(evergreen.APIUserHeader, username)
	r.Header.Add(evergreen.APIKeyHeader, key)
	resp, err := client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return task, errors.Wrap(err, "error getting task data")
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "error reading response body")
		grip.Error(err)
		return task, err
	}
	err = json.Unmarshal(body, &task)
	if err != nil {
		return task, errors.Wrap(err, "error unmarshaling json")
	}

	return task, nil
}

func makeSmokeRequest(username, key string, client *http.Client, url string) ([]byte, error) {
	grip.Infof("Getting endpoint '%s'", url)
	if !strings.HasPrefix(url, smokeUrlPrefix) {
		url = smokeUrlPrefix + smokeUiPort + url
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "error forming request")
	}
	req.Header.Add(evergreen.APIUserHeader, username)
	req.Header.Add(evergreen.APIKeyHeader, key)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "error getting endpoint '%s'", url)
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "error reading response body")
		grip.Error(err)
		return nil, err
	}

	return body, nil
}

func makeSmokeRequestAndCheck(username, key string, client *http.Client, url string, expected []string) error {
	body, err := makeSmokeRequest(username, key, client, url)
	if err != nil {
		return err
	}
	page := string(body)
	catcher := grip.NewSimpleCatcher()
	for _, text := range expected {
		if strings.Contains(page, text) {
			grip.Infof("found '%s' in endpoint '%s'", text, url)
		} else {
			logErr := fmt.Sprintf("did not find '%s' in endpoint '%s'", text, url)
			grip.Error(logErr)
			catcher.Add(errors.New(logErr))
		}
	}

	if catcher.HasErrors() {
		grip.Errorf("Failure occurred, endpoint returned: %s", body)
	}
	return catcher.Resolve()
}
