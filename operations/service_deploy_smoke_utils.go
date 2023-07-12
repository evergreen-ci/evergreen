package operations

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
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

	return errors.Errorf("Evergreen app server was not up after %d check attempts.", attempts)
}

func checkContainerTask(username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	return checkTaskStatusAndLogs(client, agent.PodMode, []string{smokeContainerTaskID}, username, key)
}

// checkHostTaskByPatch runs host tasks in the smoke test based on the project
// YAML (agent.yml) and the regexp specifying the tasks to run.
func checkHostTaskByPatch(projectName, bvToRun, cliPath, cliConfigPath, username, key string) error {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	// Triggering the repotracker causes the app server to pick up the latest
	// available mainline commits.
	if err := triggerRepotracker(projectName, username, key, client); err != nil {
		return errors.Wrap(err, "triggering repotracker to run")
	}

	// Wait until the repotracker actually pick up a commit and make a version.
	if err := waitForRepotracker(projectName, username, key, client); err != nil {
		return errors.Wrap(err, "waiting for repotracker to pick up new commits")
	}

	// Submit the smoke test manual patch.
	if err := submitSmokeTestPatch(cliPath, cliConfigPath, bvToRun); err != nil {
		return errors.Wrap(err, "submitting smoke test patch")
	}

	patchID, err := getSmokeTestPatch(projectName, username, key, client)
	if err != nil {
		return errors.Wrap(err, "getting smoke test patch")
	}

	// Check that the builds are created after submitting the patch.
	builds, err := getAndCheckBuilds(patchID, username, key, client)
	if err != nil {
		return errors.Wrap(err, "getting and checking builds")
	}

	// Check that the tasks eventually finish and check their output.
	if err = checkTaskStatusAndLogs(client, agent.HostMode, builds[0].Tasks, username, key); err != nil {
		return errors.Wrap(err, "checking task statuses and logs")
	}

	// Now that the task generator has run, check the builds again for the
	// generated tasks.
	originalTasks := builds[0].Tasks
	builds, err = getAndCheckBuilds(patchID, username, key, client)
	if err != nil {
		return errors.Wrap(err, "getting and checking builds after generating tasks")
	}

	// Isolate the generated tasks from the original tasks.
	_, generatedTasks := utility.StringSliceSymmetricDifference(originalTasks, builds[0].Tasks)
	if len(generatedTasks) == 0 {
		return errors.Errorf("no tasks were generated, expected at least one task to be generated")
	}
	// Check that the generated tasks eventually finish and check their output.
	return checkTaskStatusAndLogs(client, agent.HostMode, generatedTasks, username, key)
}

// triggerRepotracker makes a request to the Evergreen app server's REST API to
// run the repotracker. This is a necessary prerequisite to catch the smoke
// test's mainline commits up to the latest version. That way, when the smoke
// test submits a manual patch, the total diff size against the base commit is
// not so large.
// Note that this returning success means that the repotracker will run
// eventually. It does *not* guarantee that it has already run, nor that it has
// actually managed to pick up the latest commits from GitHub.
func triggerRepotracker(projectName, username, key string, client *http.Client) error {
	grip.Info("Attempting to trigger repotracker to run.")

	const repotrackerAttempts = 5
	for i := 0; i < repotrackerAttempts; i++ {
		time.Sleep(2 * time.Second)
		grip.Infof("Requesting repotracker for evergreen project. (%d/%d)", i+1, repotrackerAttempts)
		_, err := makeSmokeRequest(username, key, http.MethodPost, client, fmt.Sprintf("/rest/v2/projects/%s/repotracker", projectName))
		if err != nil {
			grip.Error(errors.Wrap(err, "requesting repotracker to run"))
			continue
		}

		grip.Info("Successfully triggered repotracker to run.")
		return nil
	}

	return errors.Errorf("could not successfully trigger repotracker after %d attempts", repotrackerAttempts)
}

// waitForRepotracker waits for the repotracker to pick up new commits and
// create versions for them. The particular versions that it creates for these
// commits is not that important, only that they exist.
func waitForRepotracker(projectName, username, key string, client *http.Client) error {
	grip.Info("Waiting for repotracker to pick up new commits.")

	const repotrackerAttempts = 10
	for i := 0; i < repotrackerAttempts; i++ {
		time.Sleep(2 * time.Second)
		respBody, err := makeSmokeRequest(username, key, http.MethodGet, client, fmt.Sprintf("/rest/v2/projects/%s/versions?limit=1", projectName))
		if err != nil {
			grip.Error(errors.Wrapf(err, "requesting latest version for project '%s'", projectName))
			continue
		}
		if len(respBody) == 0 {
			grip.Error(errors.Errorf("did not find any latest revisions yet for project '%s'", projectName))
			continue
		}

		// The repotracker should pick up new commits and create versions;
		// therefore, the version creation time should be just a few moments
		// ago.
		latestVersions := []model.APIVersion{}
		if err := json.Unmarshal(respBody, &latestVersions); err != nil {
			grip.Error(errors.Wrap(err, "reading version create time from response body"))
			continue
		}
		if len(latestVersions) == 0 {
			grip.Error(errors.Errorf("listing latest versions for project '%s' yielded no results", projectName))
			continue
		}

		latestVersion := latestVersions[0]
		latestVersionID := utility.FromStringPtr(latestVersion.Id)
		if createTime := utility.FromTimePtr(latestVersion.CreateTime); time.Now().Sub(createTime) > 365*24*time.Hour {
			grip.Infof("Found latest version '%s' for project '%s', but it was created at %s, which was a long time ago, waiting for repotracker to pick up newer commit.", latestVersionID, projectName, createTime)
			continue
		}

		grip.Infof("Repotracker successfully picked up a new commit '%s' and created version '%s'.", utility.FromStringPtr(latestVersion.Revision), latestVersionID)
		return nil
	}

	return errors.Errorf("timed out waiting for repotracker to pick up new commits and create versions after %d attempts", repotrackerAttempts)
}

// submitSmokeTestPatch submits a manual patch to the app server to run the
// smoke test and selects tasks in the given build variant.
// Note that this requires using the CLI because there's currently no way to
// create a patch from the REST API.
func submitSmokeTestPatch(cliPath, cliConfigPath, bvToRun string) error {
	grip.Info("Submitting patch to smoke test app server.")

	exit := make(chan error, 1)
	wd, err := os.Getwd()
	if err != nil {
		return errors.Wrap(err, "getting working directory")
	}
	if err := smokeRunBinary(exit, "smoke-patch-submission", wd, cliPath, "-c", cliConfigPath, "patch", "-p", "evergreen", "-v", bvToRun, "-t", "all", "-f", "-y", "-d", "Smoke test patch"); err != nil {
		return errors.Wrap(err, "starting Evergreen CLI to submit patch")
	}
	if err := <-exit; err != nil {
		return errors.Wrap(err, "running Evergreen CLI to submit patch")
	}

	grip.Info("Successfully submitted patch to smoke test app server.")
	return nil
}

// getSmokeTestPatch gets the user's manual patch that was submitted to the app
// server. It returns the patch ID.
func getSmokeTestPatch(projectName, username, key string, client *http.Client) (string, error) {
	grip.Infof("Waiting for manual patch for user '%s' to exist.", username)

	const patchCheckAttempts = 10
	for i := 0; i < patchCheckAttempts; i++ {
		time.Sleep(2 * time.Second)
		respBody, err := makeSmokeRequest(username, key, http.MethodGet, client, fmt.Sprintf("/rest/v2/users/%s/patches?limit=1", username))
		if err != nil {
			grip.Error(errors.Wrapf(err, "requesting latest patches for user '%s'", username))
			continue
		}
		if len(respBody) == 0 {
			grip.Error(errors.Errorf("did not find any latest patches yet for user '%s'", username))
			continue
		}

		// The new patch should exist, and the latest user patch creation time
		// should be just a few moments ago.
		latestPatches := []model.APIPatch{}
		if err := json.Unmarshal(respBody, &latestPatches); err != nil {
			grip.Error(errors.Wrap(err, "reading response body"))
			continue
		}
		if len(latestPatches) == 0 {
			grip.Error(errors.Errorf("listing latest patches for user '%s' yielded no results", username))
			continue
		}

		latestPatch := latestPatches[0]
		latestPatchID := utility.FromStringPtr(latestPatch.Id)
		if createTime := utility.FromTimePtr(latestPatch.CreateTime); time.Now().Sub(createTime) > time.Hour {
			grip.Infof("Found latest patch '%s' in project '%s', but it was created at %s, waiting for patch that was just submitted", latestPatchID, utility.FromStringPtr(latestPatch.ProjectId), createTime)
			continue
		}

		grip.Infof("Successfully found patch '%s' in project '%s' submitted by user '%s'.", latestPatchID, projectName, username)
		return latestPatchID, nil
	}

	return "", errors.Errorf("timed out waiting for repotracker to pick up new commits and create versions after %d attempts", patchCheckAttempts)
}

// getAndCheckBuilds gets build and task information from the Evergreen app
// server's REST API for the smoke test's manual patch.
func getAndCheckBuilds(patchID, username, key string, client *http.Client) ([]apimodels.APIBuild, error) {
	grip.Infof("Attempting to get builds created by the manual patch '%s'.", patchID)

	const buildCheckAttempts = 10
	for i := 0; i < buildCheckAttempts; i++ {
		// Poll the app server until the patch's builds and tasks exist.
		time.Sleep(2 * time.Second)

		grip.Infof("Checking for a build of patch '%s'. (%d/%d)", patchID, i+1, buildCheckAttempts)
		body, err := makeSmokeRequest(username, key, http.MethodGet, client, fmt.Sprintf("/rest/v2/versions/%s/builds", patchID))
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

		grip.Infof("Successfully got %d build(s) for manual patch '%s'.", len(builds), patchID)
		return builds, nil
	}

	return nil, errors.Errorf("could not get builds after %d attempts - this might indicate that there was an issue in the repotracker that prevented the creation of builds", buildCheckAttempts)
}

// checkTaskStatusAndLogs checks that all the expected tasks are finished,
// succeeded, and performed the expected operations based on the task log
// contents.
func checkTaskStatusAndLogs(client *http.Client, mode agent.Mode, tasks []string, username, key string) error {
	grip.Infof("Checking task status and task logs for tasks: %s", strings.Join(tasks, ", "))

	const taskCheckAttempts = 40
	nextTaskToCheckIdx := 0
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
		grip.Infof("Checking %d remaining task(s). (%d/%d)", len(tasks)-nextTaskToCheckIdx, i+1, taskCheckAttempts)

		for i, taskID := range tasks[nextTaskToCheckIdx:] {
			task, err := getTaskInfo(client, username, key, taskID)
			if err != nil {
				return errors.WithStack(err)
			}

			if !evergreen.IsFinishedTaskStatus(task.Status) {
				grip.Infof("Found task '%s' is not yet finished and has status '%s' (expected '%s').", taskID, task.Status, evergreen.TaskSucceeded)
				continue OUTER
			}
			if task.Status != evergreen.TaskSucceeded {
				return errors.Errorf("finished task '%s' has non-successful status '%s' (expected '%s')", taskID, task.Status, evergreen.TaskSucceeded)
			}
			if err = getAndCheckTaskLog(task, client, mode, username, key); err != nil {
				return errors.Wrapf(err, "getting and checking task log for task '%s'", taskID)
			}

			nextTaskToCheckIdx = i
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
		grip.Info("Found expected task completed message in task log.")
	} else {
		return errors.New("did not find task completed message in task logs")
	}

	// Note that these checks have a direct dependency on the task configuration
	// in the smoke test's project YAML (agent.yml).
	if mode == agent.HostMode {
		const generatorTaskName = "smoke_test_generate_task"
		if strings.Contains(page, generatorTaskName) {
			if !strings.Contains(page, "Finished command 'generate.tasks'") {
				return errors.New("did not find expected log in generate.tasks command")
			}
			return nil
		}
		if strings.Contains(page, "task_to_add_via_generator") {
			if !strings.Contains(page, "generated_task") {
				return errors.New("did not find expected log in generated task")
			}
			return nil
		}
		// Validate that setup_group only runs in first task
		const firstTaskGroupTaskLog = "smoke test is running the first task in the task group"
		const setupGroupLog = "smoke test is running the setup group"
		if strings.Contains(page, firstTaskGroupTaskLog) {
			if !strings.Contains(page, setupGroupLog) {
				return errors.New("did not find setup_group in task logs for first task")
			}
		} else {
			if strings.Contains(page, setupGroupLog) {
				return errors.New("setup_group should only run in first task")
			}
		}

		// Validate that setup_task and teardown_task run for all tasks
		const setupTaskLog = "smoke test is running the setup task"
		if !strings.Contains(page, setupTaskLog) {
			return errors.New("did not find setup_task in task logs")
		}
		const teardownTaskLog = "smoke test is running the teardown task"
		if !strings.Contains(page, teardownTaskLog) {
			return errors.New("did not find teardown_task in task logs")
		}

		// Validate that teardown_group only runs in last task
		const lastTaskGroupTaskLog = "smoke test is running the fourth task in the task group"
		const teardownGroupLog = "smoke test is running the teardown group"
		if strings.Contains(page, lastTaskGroupTaskLog) {
			if !strings.Contains(page, teardownGroupLog) {
				return errors.New("did not find teardown_group in task logs for last task in the task group")
			}
		} else {
			if strings.Contains(page, teardownGroupLog) {
				return errors.New("teardown_group should only run in last task in the task group")
			}
		}
	} else if mode == agent.PodMode {
		// TODO (PM-2617) Add task groups to the container task smoke test once they are supported
		// TODO (EVG-17658): this test is highly fragile and has a chance of
		// breaking if you change any of the container task YAML setup (e.g.
		// change any container-related configuration in agent.yml). If you
		// change the container task setup, you will likely also have to change
		// the smoke testdata. EVG-17658 should address the issues with the
		// smoke test.
		const containerTaskLog = "container task"
		if !strings.Contains(page, containerTaskLog) {
			return errors.Errorf("did not find expected container task log: '%s'", containerTaskLog)
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

	body, err := io.ReadAll(resp.Body)
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

	body, err := io.ReadAll(resp.Body)
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
