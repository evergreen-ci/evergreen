package operations

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
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

func (tests smokeEndpointTestDefinitions) checkEndpoints() error {
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)
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
	for url, expected := range tests.UI {
		grip.Infof("Getting endpoint '%s'", url)
		resp, err := client.Get(smokeUrlPrefix + smokeUiPort + url)
		if err != nil {
			catcher.Add(errors.Errorf("error getting UI endpoint '%s'", url))
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		page := string(body)
		for _, text := range expected {
			if strings.Contains(page, text) {
				grip.Infof("found '%s' in UI endpoint '%s'", text, url)
			} else {
				grip.Infof("did not find '%s' in UI endpoint '%s'", text, url)
				catcher.Add(errors.Errorf("'%s' not in UI endpoint '%s'", text, url))
			}
		}
	}

	grip.InfoWhen(!catcher.HasErrors(), "success: all endpoints accessible")

	return errors.Wrapf(catcher.Resolve(), "failed to get %d endpoints", catcher.Len())
}

func checkTaskByCommit(username, key, commit string) error {
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	var builds []apimodels.APIBuild
	var build apimodels.APIBuild
	for i := 0; i <= 300; i++ {
		// get task id
		if i == 300 {
			return errors.New("error getting builds for version")
		}
		time.Sleep(time.Second)
		grip.Infof("checking for a build of %s (%d/300)", commit, i+1)
		resp, err := client.Get(smokeUrlPrefix + smokeUiPort + "/rest/v2/versions/evergreen_" + commit + "/builds")
		if err != nil {
			grip.Info(err)
			continue
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
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
			grip.Info("no tasks found")
			continue
		}
		break
	}

	var task apimodels.APITask
	for i := 0; i <= 300; i++ {
		// check task
		if i == 300 {
			return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
		}
		time.Sleep(time.Second)
		grip.Infof("checking for task %s (%d/300)", builds[0].Tasks[0], i+1)
		r, err := http.NewRequest("GET", smokeUrlPrefix+smokeUiPort+"/rest/v2/tasks/"+builds[0].Tasks[0], nil)
		if err != nil {
			return errors.Wrap(err, "failed to make request")
		}
		r.Header.Add(evergreen.APIUserHeader, username)
		r.Header.Add(evergreen.APIKeyHeader, key)
		resp, err := client.Do(r)
		if err != nil {
			return errors.Wrap(err, "error getting task data")
		}
		defer resp.Body.Close()
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			err = errors.Wrap(err, "error reading response body")
			grip.Error(err)
			return err
		}
		err = json.Unmarshal(body, &task)
		if err != nil {
			return errors.Wrap(err, "error unmarshaling json")
		}

		if task.Status != evergreen.TaskSucceeded {
			grip.Infof("found task is status %s", task.Status)
			continue
		}
		break
	}
	grip.Infof("checking for log %s", task.Logs["task_log"])
	resp, err := client.Get(task.Logs["task_log"] + "&text=true")
	if err != nil {
		return errors.Wrap(err, "error getting log data")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "error reading response body")
		return err
	}
	page := string(body)
	if strings.Contains(page, "Task completed - SUCCESS") {
		grip.Infof("Found task completed message in log:\n%s", page)
	} else {
		grip.Errorf("did not find task completed message in log:\n%s", page)
		return errors.New("did not find task completed message in log")
	}

	grip.Info("Successfully checked task by commit")
	return nil
}
