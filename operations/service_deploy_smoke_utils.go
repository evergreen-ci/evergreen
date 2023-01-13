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

func (td smokeEndpointTestDefinitions) checkEndpoints(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)
	client.Timeout = time.Second

	// wait for web service to start
	if err := td.waitForEvergreen(client); err != nil {
		return errors.Wrap(err, "waiting for Evergreen to be up")
	}

	// check endpoints
	catcher := grip.NewSimpleCatcher()
	grip.Info("Testing UI Endpoints")
	for url, expected := range td.UI {
		catcher.Wrap(makeSmokeGetRequestAndCheck(username, key, client, url, expected), "testing UI endpoints")
	}

	grip.Info("Testing API Endpoints")
	for url, expected := range td.API {
		catcher.Wrap(makeSmokeGetRequestAndCheck(username, key, client, "/api"+url, expected), "testing API endpoints")
	}

	grip.InfoWhen(!catcher.HasErrors(), "Success: all endpoints are accessible.")

	return errors.Wrapf(catcher.Resolve(), "testing endpoints")
}

// waitForEvergreen waits for the Evergreen app server to be up and accepting
// requests.
func (td smokeEndpointTestDefinitions) waitForEvergreen(client *http.Client) error {
	const attempts = 10
	for i := 0; i < attempts; i++ {
		grip.Infof("Checking if Evergreen is up. (%d/%d)", i, attempts)
		if _, err := client.Get(smokeUrlPrefix + smokeUiPort); err != nil {
			grip.Error(errors.Wrap(err, "connecting to Evergreen"))
			time.Sleep(time.Second)
			continue
		}

		grip.Info("Evergreen is up.")
		return nil
	}

	return errors.Errorf("Evergreen was not up after %d check attempts.", attempts)
}

// getLatestGithubCommit gets the latest commit on the main branch of Evergreen.
// The smoke test is implicitly assuming here that the commit the repotracker
// picks up is the latest commit on the main branch of Evergreen.
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

	return checkTaskStatusAndLogs(client, agent.PodMode, []string{smokeContainerTaskID}, username, key)
}

func checkHostTaskByCommit(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	// Triggering the repotracker causes the app server to pick up the latest
	// available commits and create versions, builds, and tasks for that commit.
	if err := triggerRepotracker(username, key, client); err != nil {
		return errors.Wrap(err, "triggering repotracker to run")
	}

	// Check that the builds should be eventually created after triggering the
	// repotracker.
	builds, err := getAndCheckBuilds(username, key, client)
	if err != nil {
		return errors.Wrap(err, "getting and checking builds")
	}

	// Check that the tasks eventually run after triggering the repotracker.
	return checkTaskStatusAndLogs(client, agent.HostMode, builds[0].Tasks, username, key)
}

// triggerRepotracker makes a request to the Evergreen app server's REST API to
// run the repotracker. Note that this returning success means that the
// repotracker will run eventually. It does *not* guarantee that it has already
// run, nor that it has actually managed to pick up the latest commits from
// GitHub.
func triggerRepotracker(username, key string, client *http.Client) error {
	grip.Info("Attempting to trigger repotracker to run.")

	const repotrackerAttempts = 5
	for i := 0; i < repotrackerAttempts; i++ {
		time.Sleep(2 * time.Second)
		grip.Infof("Requesting repotracker for evergreen project. (%d/%d)", i+1, repotrackerAttempts)
		_, err := makeSmokeRequest(username, key, http.MethodPost, client, "/rest/v2/projects/evergreen/repotracker")
		if err != nil {
			grip.Error(errors.Wrap(err, "requesting repotracker to run"))
			continue
		}

		grip.Info("Successfully triggered repotracker to run.")
		return nil
	}

	return errors.Errorf("could not successfully trigger repotracker after %d attempts", repotrackerAttempts)
}

// getAndCheckBuilds gets build information from the Evergreen app server's REST
// API for the builds that it expects to be eventaully created. These checks are
// assuming that, by triggering the repotracker to run, a version should
// eventually be created for the latest commit to Evergreen.
func getAndCheckBuilds(username, key string, client *http.Client) ([]apimodels.APIBuild, error) {
	grip.Info("Attempting to get builds created by triggering the repotracker.")

	const buildCheckAttempts = 30
	for i := 0; i < buildCheckAttempts; i++ {
		// Poll the app server until the builds exist and have tasks.
		// This is implicitly assuming that the app server has proper
		// integration with GitHub to pick up the latest commit and successfully
		// creates versions, builds, and tasks based on that commit.

		time.Sleep(10 * time.Second)

		// The latest GitHub commit must match the one that the repotracker
		// picks up.
		latest, err := getLatestGithubCommit()
		if err != nil {
			grip.Error(errors.Wrap(err, "getting latest GitHub commit"))
			continue
		}
		grip.Infof("Checking for a build of commit '%s'. (%d/%d)", latest, i+1, buildCheckAttempts)
		body, err := makeSmokeRequest(username, key, http.MethodGet, client, "/rest/v2/versions/evergreen_"+latest+"/builds")
		if err != nil {
			grip.Error(errors.Wrap(err, "requesting builds"))
			continue
		}

		builds := []apimodels.APIBuild{}
		err = json.Unmarshal(body, &builds)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshalling JSON response body into builds")
		}
		if len(builds) == 0 {
			continue
		}
		if len(builds[0].Tasks) == 0 {
			continue
		}

		grip.Infof("Successfully got %d builds for commit '%s' created by triggering the repotracker.", len(builds), latest)
		return builds, nil
	}

	return nil, errors.Errorf("could not get builds after %d attempts - this might indicate that there was an issue in the repotracker that prevented the creation of builds", buildCheckAttempts)
}

// checkTaskStatusAndLogs checks that all the expected tasks are finished,
// succeeded, and performed the expected operations based on the task log
// contents.
func checkTaskStatusAndLogs(client *http.Client, mode agent.Mode, tasks []string, username, key string) error {
	grip.Infof("Checking task status and task logs for tasks: %s", strings.Join(tasks, ", "))
	const taskCheckAttempts = 30
OUTER:
	for i := 0; i < taskCheckAttempts; i++ {
		// Poll the app server until the task is finished and check its task
		// logs for the expected results.
		// It's worth noting there that there is a substantial amount of
		// heavy-lifting being done here by the Evergreen app server and agent
		// under the covers. In the background, the app server must run the
		// scheduler to create a task queue to run the new tasks, the agent must
		// pick up those tasks from that task queue, and the agent must run
		// those tasks and report the result back to the app server. If any of
		// those operations go wrong (e.g. due to a modification to the app
		// server, agent, or smoke configuration files), these checks can fail.
		time.Sleep(10 * time.Second)
		grip.Infof("Checking %d tasks. (%d/%d)", len(tasks), i+1, taskCheckAttempts)

		for _, taskId := range tasks {
			task, err := getTaskInfo(client, username, key, taskId)
			if err != nil {
				return errors.WithStack(err)
			}

			if !evergreen.IsFinishedTaskStatus(task.Status) {
				grip.Infof("Found task '%s' is not yet finished and has status '%s' (expected '%s').", taskId, task.Status, evergreen.TaskSucceeded)
				continue OUTER
			}
			if task.Status != evergreen.TaskSucceeded {
				return errors.Errorf("finished task '%s' has non-successful status '%s' (expected '%s')", taskId, task.Status, evergreen.TaskSucceeded)
			}
			if err = getAndCheckTaskLog(task, client, mode, username, key); err != nil {
				return errors.Wrapf(err, "getting and checking task log for task '%s'", taskId)
			}
		}

		grip.Infof("Successfully checked %d %s tasks and their task logs.", len(tasks), string(mode))
		return nil
	}

	return errors.Errorf("task status and task log checks were incomplete after %d attempts - this might indicate an underlying issue with the app server, the agent, or the smoke test's configuration setup", taskCheckAttempts)
}

// getAndCheckTaskLog gets the task logs from the task log URL and checks that
// it has the expected content, indicating that the task executed the commands
// properly.
func getAndCheckTaskLog(task apimodels.APITask, client *http.Client, mode agent.Mode, username, key string) error {
	// retry for *slightly* delayed logger closing
	const taskLogCheckAttempts = 3
	for i := 0; i < taskLogCheckAttempts; i++ {
		grip.Infof("Checking for task log from URL %s. (%d/%d)", task.Logs["task_log"], i+1, taskLogCheckAttempts)
		body, err := makeSmokeRequest(username, key, http.MethodGet, client, task.Logs["task_log"]+"&text=true")
		if err != nil {
			grip.Error(errors.Wrap(err, "getting task log data"))
			continue
		}
		if err := checkTaskLogContent(body, mode); err != nil {
			grip.Error(errors.Wrap(err, "getting task log data"))
			continue
		}

		return nil
	}

	return errors.Errorf("task log check failed after %d attempts", taskLogCheckAttempts)
}

// checkTaskLogContent compares the expected result of running the smoke test
// project YAML (agent.yml) against the actual task log's text.
func checkTaskLogContent(body []byte, mode agent.Mode) error {
	page := string(body)

	// Validate that task contains task completed message
	if strings.Contains(page, "Task completed - SUCCESS") {
		grip.Info("Found task completed message in task log")
	} else {
		return errors.New("did not find task completed message in task logs")
	}

	// Note that these checks have a direct dependency on the task configuration
	// in the smoke test's project YAML (agent.yml).
	if mode == agent.HostMode {
		// Validate that setup_group only runs in first task
		if strings.Contains(page, "first") {
			if !strings.Contains(page, "setup_group") {
				return errors.New("did not find setup_group in task logs for first task")
			}
		} else {
			if strings.Contains(page, "setup_group") {
				return errors.New("setup_group should only run in first task")
			}
		}

		// Validate that setup_task and teardown_task run for all tasks
		if !strings.Contains(page, "setup_task") {
			return errors.New("did not find setup_task in task logs")
		}
		if !strings.Contains(page, "teardown_task") {
			return errors.New("did not find teardown_task in task logs")
		}

		// Validate that teardown_group only runs in last task
		if strings.Contains(page, "fourth") {
			if !strings.Contains(page, "teardown_group") {
				return errors.New("did not find teardown_group in task logs for last (fourth) task")
			}
		} else {
			if strings.Contains(page, "teardown_group") {
				return errors.New("teardown_group should only run in last (fourth) task")
			}
		}
	} else if mode == agent.PodMode {
		// TODO (PM-2617) Add task groups to the container task smoke test once they are supported
		if !strings.Contains(page, "container task") {
			return errors.New("did not find container task in logs")
		}
	}

	return nil
}

// getTaskInfo gets basic information about the current status and task logs for
// the given task ID from the REST API.
func getTaskInfo(client *http.Client, username, key string, taskId string) (apimodels.APITask, error) {
	grip.Infof("Checking information for task '%s'.", taskId)

	task := apimodels.APITask{}
	r, err := http.NewRequest(http.MethodGet, smokeUrlPrefix+smokeUiPort+"/rest/v2/tasks/"+taskId, nil)
	if err != nil {
		return task, errors.Wrap(err, "making request for task")
	}
	r.Header.Add(evergreen.APIUserHeader, username)
	r.Header.Add(evergreen.APIKeyHeader, key)

	resp, err := client.Do(r)
	if err != nil {
		return task, errors.Wrap(err, "getting task data")
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return task, errors.Wrap(err, "reading response body")
	}
	if resp.StatusCode >= 400 {
		return task, errors.Errorf("got HTTP response code %d with error %s", resp.StatusCode, string(body))
	}

	if err := json.Unmarshal(body, &task); err != nil {
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
	if err != nil {
		return nil, errors.Wrapf(err, "getting endpoint '%s'", url)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "reading response body")
	}
	if resp.StatusCode >= 400 {
		return body, errors.Errorf("got HTTP response code %d with error %s", resp.StatusCode, string(body))
	}

	return body, nil
}

func makeSmokeGetRequestAndCheck(username, key string, client *http.Client, url string, expected []string) error {
	body, err := makeSmokeRequest(username, key, http.MethodGet, client, url)
	grip.Error(errors.Wrap(err, "making smoke request"))
	page := string(body)
	catcher := grip.NewSimpleCatcher()
	for _, text := range expected {
		if strings.Contains(page, text) {
			grip.Infof("Successfully found expected text '%s' from endpoint '%s'.", text, url)
		} else {
			catcher.Errorf("did not find '%s' in endpoint '%s'", text, url)
		}
	}

	if catcher.HasErrors() {
		grip.Errorf("Failure occurred, endpoint returned: %s", body)
	}
	return catcher.Resolve()
}
