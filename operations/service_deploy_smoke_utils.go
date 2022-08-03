package operations

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v34/github"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	// uiPort is the local port the UI will listen on.
	smokeUiPort = ":9090"
	// urlPrefix is the localhost prefix for accessing local Evergreen.
	smokeUrlPrefix       = "http://localhost"
	smokeContainerTaskID = "evergreen_pod_bv_container_task_a71e20e60918bb97d41422e94d04822be2a22e8e_22_08_22_13_44_49"
)

// smokeEndpointTestDefinitions describes the UI and API endpoints to verify are up.
type smokeEndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

func (tests smokeEndpointTestDefinitions) checkEndpoints(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)
	client.Timeout = time.Second

	// wait for web service to start
	attempts := 10
	for i := 1; i <= attempts; i++ {
		grip.Infof("checking if Evergreen is up (attempt %d of %d)", i, attempts)
		_, err := client.Get(smokeUrlPrefix + smokeUiPort)
		if err != nil {
			if i == attempts {
				err = errors.Wrapf(err, "connecting to Evergreen after %d attempts", attempts)
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
		catcher.Wrap(makeSmokeGetRequestAndCheck(username, key, client, url, expected), "testing UI endpoints")
	}

	grip.Info("Testing API Endpoints")
	for url, expected := range tests.API {
		catcher.Wrap(makeSmokeGetRequestAndCheck(username, key, client, "/api"+url, expected), "testing API endpoints")
	}

	grip.InfoWhen(!catcher.HasErrors(), "success: all endpoints accessible")

	return errors.Wrapf(catcher.Resolve(), "testing endpoints")
}

func getLatestGithubCommit() (string, error) {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	resp, err := client.Get("https://api.github.com/repos/evergreen-ci/evergreen/git/refs/heads/main")
	if err != nil {
		return "", errors.Wrap(err, "getting latest commit from GitHub")
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "reading response body from GitHub")
	}

	latest := github.Reference{}
	if err = json.Unmarshal(body, &latest); err != nil {
		return "", errors.Wrap(err, "unmarshalling response from GitHub")
	}
	if latest.Object != nil && latest.Object.SHA != nil && *latest.Object.SHA != "" {
		return *latest.Object.SHA, nil
	}
	return "", errors.New("could not find latest commit in response")
}

func checkContainerTask(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	return checkForTask(client, agent.PodMode, []string{smokeContainerTaskID}, username, key)
}

func checkHostTaskByCommit(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	var builds []apimodels.APIBuild
	var build apimodels.APIBuild

	// trigger repotracker to insert relevant builds and tasks from agent.yml definitions
	for i := 0; i < 5; i++ {
		if i == 5 {
			return errors.Errorf("unable to trigger the repotracker after 5 attempts")
		}
		time.Sleep(2 * time.Second)
		grip.Infof("running repotracker for evergreen project (%d/5)", i)
		_, err := makeSmokeRequest(username, key, http.MethodPost, client, "/rest/v2/projects/evergreen/repotracker")
		if err != nil {
			grip.Error(err)
			continue
		}
		break
	}
	for i := 0; i <= 30; i++ {
		// get task id
		if i == 30 {
			return errors.New("ran out of attempts to get builds for version")
		}
		time.Sleep(10 * time.Second)
		latest, err := getLatestGithubCommit()
		if err != nil {
			grip.Error(errors.Wrap(err, "getting latest GitHub commit"))
			continue
		}
		grip.Infof("checking for a build of %s (%d/30)", latest, i+1)
		body, err := makeSmokeRequest(username, key, http.MethodGet, client, "/rest/v2/versions/evergreen_"+latest+"/builds")
		if err != nil {
			grip.Error(err)
			continue
		}

		err = json.Unmarshal(body, &builds)
		if err != nil {
			err = json.Unmarshal(body, &build)
			if err != nil {
				return errors.Wrap(err, "unmarshalling JSON response body into builds")
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
	return checkForTask(client, agent.HostMode, builds[0].Tasks, username, key)
}

func checkForTask(client *http.Client, mode agent.Mode, tasks []string, username, key string) error {
	var taskStatus string
OUTER:
	for i := 0; i <= 30; i++ {
		// check task
		if i == 30 {
			return errors.Errorf("task status is %s (expected %s)", taskStatus, evergreen.TaskSucceeded)
		}
		time.Sleep(10 * time.Second)
		grip.Infof("checking for %d tasks (%d/30)", len(tasks), i+1)

		for _, taskId := range tasks {
			task, err := checkTask(client, username, key, taskId)
			if err != nil {
				return errors.WithStack(err)
			}

			if task.Status == evergreen.TaskFailed {
				return errors.Errorf("task status is %s (expected %s)", task.Status, evergreen.TaskSucceeded)
			}
			if task.Status != evergreen.TaskSucceeded {
				grip.Infof("found task is status %s", task.Status)
				taskStatus = task.Status
				continue OUTER
			}
			err = checkLog(task, client, mode, username, key)
			if err != nil {
				return err
			}
		}
		grip.Infof("Successfully checked %d %s tasks", len(tasks), string(mode))
		return nil
	}
	return errors.New("this code should be unreachable")
}

func checkLog(task apimodels.APITask, client *http.Client, mode agent.Mode, username, key string) error {
	// retry for *slightly* delayed logger closing
	var err error
	for i := 0; i < 3; i++ {
		grip.Infof("checking for log %s (%d/3)", task.Logs["task_log"], i+1)
		var body []byte
		body, err = makeSmokeRequest(username, key, http.MethodGet, client, task.Logs["task_log"]+"&text=true")
		if err != nil {
			err = errors.Wrap(err, "error getting log data")
			grip.Debug(err)
			continue
		}
		if err = checkTaskLog(body, mode); err == nil {
			break
		}
	}
	return err
}

func checkTaskLog(body []byte, mode agent.Mode) error {
	page := string(body)

	// Validate that task contains task completed message
	if strings.Contains(page, "Task completed - SUCCESS") {
		grip.Infof("Found task completed message in log:\n%s", page)
	} else {
		grip.Errorf("did not find task completed message in log:\n%s", page)
		return errors.New("did not find task completed message in log")
	}

	if mode == agent.HostMode {
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
	} else if mode == agent.PodMode {
		// TODO (PM-2617) Add task groups to the container task smoke test once they aree supported
		if !strings.Contains(page, "container task") {
			return errors.New("did not find container task in logs")
		}
	}

	return nil
}

func checkTask(client *http.Client, username, key string, taskId string) (apimodels.APITask, error) {
	task := apimodels.APITask{}
	grip.Infof("checking for task %s", taskId)
	r, err := http.NewRequest(http.MethodGet, smokeUrlPrefix+smokeUiPort+"/rest/v2/tasks/"+taskId, nil)
	if err != nil {
		return task, errors.Wrap(err, "making request for task")
	}
	r.Header.Add(evergreen.APIUserHeader, username)
	r.Header.Add(evergreen.APIKeyHeader, key)
	resp, err := client.Do(r)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return task, errors.Wrap(err, "getting task data")
	}
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "reading response body")
		grip.Error(err)
		return task, err
	}
	if resp.StatusCode >= 400 {
		return task, errors.Errorf("got HTTP response code %d with error %s", resp.StatusCode, string(body))
	}

	err = json.Unmarshal(body, &task)
	if err != nil {
		return task, errors.Wrap(err, "unmarshalling JSON response body into task")
	}

	return task, nil
}

func makeSmokeRequest(username, key string, method string, client *http.Client, url string) ([]byte, error) {
	grip.Infof("Getting endpoint '%s'", url)
	if !strings.HasPrefix(url, smokeUrlPrefix) {
		url = smokeUrlPrefix + smokeUiPort + url
	}
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "making request for URL '%s'", url)
	}
	req.Header.Add(evergreen.APIUserHeader, username)
	req.Header.Add(evergreen.APIKeyHeader, key)
	resp, err := client.Do(req)
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrapf(err, "getting endpoint '%s'", url)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		err = errors.Wrap(err, "reading response body")
		grip.Error(err)
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return body, errors.Errorf("got HTTP response code %d with error %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func makeSmokeGetRequestAndCheck(username, key string, client *http.Client, url string, expected []string) error {
	body, err := makeSmokeRequest(username, key, http.MethodGet, client, url)
	grip.Error(err)
	page := string(body)
	catcher := grip.NewSimpleCatcher()
	for _, text := range expected {
		if strings.Contains(page, text) {
			grip.Infof("found '%s' in endpoint '%s'", text, url)
		} else {
			logErr := errors.Errorf("did not find '%s' in endpoint '%s'", text, url)
			grip.Error(logErr)
			catcher.Add(logErr)
		}
	}

	if catcher.HasErrors() {
		grip.Errorf("Failure occurred, endpoint returned: %s", body)
	}
	return catcher.Resolve()
}
