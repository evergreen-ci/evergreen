package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/require"
)

// SmokeTestParams contain common parameters used by smoke tests.
type SmokeTestParams struct {
	internal.APIParams
	ExecModeID     string
	ExecModeSecret string
	CLIConfigPath  string
	ProjectID      string
	BVName         string
}

// GetSmokeTestParamsFromEnv gets the parameters for the host smoke test from
// the environment. It sets defaults where possible. Note that the default data
// depends on the setup test data for the smoke test.
func GetSmokeTestParamsFromEnv(t *testing.T) SmokeTestParams {
	evgHome := evergreen.FindEvergreenHome()
	require.NotZero(t, evgHome, "EVGHOME must be set for smoke test")

	execModeID := os.Getenv("EXEC_MODE_ID")
	if execModeID == "" {
		execModeID = "localhost"
	}

	execModeSecret := os.Getenv("EXEC_MODE_SECRET")
	if execModeSecret == "" {
		execModeSecret = "de249183582947721fdfb2ea1796574b"
	}

	cliConfigPath := os.Getenv("CLI_CONFIG_PATH")
	if cliConfigPath == "" {
		cliConfigPath = filepath.Join(evgHome, "smoke", "internal", "testdata", "cli.yml")
	}

	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		projectID = "evergreen"
	}

	bvName := os.Getenv("BV_NAME")
	if bvName == "" {
		bvName = "localhost"
	}

	return SmokeTestParams{
		APIParams:      internal.GetAPIParamsFromEnv(t, evgHome),
		ExecModeID:     execModeID,
		ExecModeSecret: execModeSecret,
		ProjectID:      projectID,
		BVName:         bvName,
		CLIConfigPath:  cliConfigPath,
	}
}

// RunHostTaskPatchTest runs host tasks in the smoke test based on the project
// YAML (project.yml) and the regexp specifying the tasks to run.
func RunHostTaskPatchTest(ctx context.Context, t *testing.T, params SmokeTestParams) {
	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)

	// Wait until Evergreen is accepting requests.
	internal.WaitForEvergreen(t, params.AppServerURL, client)

	// Triggering the repotracker causes the app server to pick up the latest
	// available mainline commits.
	triggerRepotracker(ctx, t, params, client)

	// Wait until the repotracker actually pick up a commit and make a version.
	waitForRepotracker(ctx, t, params, client)

	// Submit the smoke test manual patch.
	submitSmokeTestPatch(ctx, t, params)

	patchID := getSmokeTestPatch(ctx, t, params, client)

	// Check that the builds are created after submitting the patch.
	builds, err := getAndCheckBuilds(ctx, params, patchID, client)
	require.NoError(t, err, "should have found builds to check")
	require.NotEmpty(t, builds, "must have at least one build to test")

	// Check that the tasks eventually finish and check their output.
	internal.CheckTaskStatusAndLogs(ctx, t, params.APIParams, client, agent.HostMode, builds[0].Tasks)

	// Now that the task generator has run, check the builds again for the
	// generated tasks.
	grip.Info("Successfully checked non-generated tasks, checking generated tasks")

	originalTasks := builds[0].Tasks
	builds, err = getAndCheckBuilds(ctx, params, patchID, client)
	require.NoError(t, err, "should have found builds to check after generating tasks")
	require.NotEmpty(t, builds, "must have at least one build to test after generating tasks")

	// Isolate the generated tasks from the original tasks.
	_, generatedTasks := utility.StringSliceSymmetricDifference(originalTasks, builds[0].Tasks)
	require.NotEmpty(t, generatedTasks, "should have generated at least one task")

	// Check that the generated tasks eventually finish and check their output.
	internal.CheckTaskStatusAndLogs(ctx, t, params.APIParams, client, agent.HostMode, generatedTasks)
}

// triggerRepotracker makes a request to the Evergreen app server's REST API to
// run the repotracker. This is a necessary prerequisite to catch the smoke
// test's mainline commits up to the latest version. That way, when the smoke
// test submits a manual patch, the total diff size against the base commit is
// not so large.
// Note that this returning success means that the repotracker will run
// eventually. It does *not* guarantee that it has already run, nor that it has
// actually managed to pick up the latest commits from GitHub.
func triggerRepotracker(ctx context.Context, t *testing.T, params SmokeTestParams, client *http.Client) {
	grip.Info("Attempting to trigger repotracker to run.")

	const repotrackerAttempts = 5
	for i := 0; i < repotrackerAttempts; i++ {
		time.Sleep(2 * time.Second)
		grip.Infof("Requesting repotracker for evergreen project. (%d/%d)", i+1, repotrackerAttempts)
		_, err := internal.MakeSmokeRequest(ctx, params.APIParams, http.MethodPost, client, fmt.Sprintf("/rest/v2/projects/%s/repotracker", params.ProjectID))
		if err != nil {
			grip.Error(errors.Wrap(err, "requesting repotracker to run"))
			continue
		}

		grip.Info("Successfully triggered repotracker to run.")

		return
	}

	require.FailNow(t, "ran out of attempts to trigger repotracker", "could not successfully trigger repotracker after %d attempts", repotrackerAttempts)
}

// waitForRepotracker waits for the repotracker to pick up new commits and
// create versions for them. The particular versions that it creates for these
// commits is not that important, only that they exist.
func waitForRepotracker(ctx context.Context, t *testing.T, params SmokeTestParams, client *http.Client) {
	grip.Info("Waiting for repotracker to pick up new commits.")

	const repotrackerAttempts = 10
	for i := 0; i < repotrackerAttempts; i++ {
		time.Sleep(2 * time.Second)
		respBody, err := internal.MakeSmokeRequest(ctx, params.APIParams, http.MethodGet, client, fmt.Sprintf("/rest/v2/projects/%s/versions?limit=1", params.ProjectID))
		if err != nil {
			grip.Error(errors.Wrapf(err, "requesting latest version for project '%s'", params.ProjectID))
			continue
		}
		if len(respBody) == 0 {
			grip.Error(errors.Errorf("did not find any latest revisions yet for project '%s'", params.ProjectID))
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
			grip.Errorf("listing latest versions for project '%s' yielded no results", params.ProjectID)
			continue
		}

		latestVersion := latestVersions[0]
		latestVersionID := utility.FromStringPtr(latestVersion.Id)
		if createTime := utility.FromTimePtr(latestVersion.CreateTime); time.Since(createTime) > 365*24*time.Hour {
			grip.Infof("Found latest version '%s' for project '%s', but it was created at %s, which was a long time ago, waiting for repotracker to pick up newer commit.", latestVersionID, params.ProjectID, createTime)
			continue
		}

		grip.Infof("Repotracker successfully picked up a new commit '%s' and created version '%s'.", utility.FromStringPtr(latestVersion.Revision), latestVersionID)

		return
	}

	require.FailNow(t, "ran out of attempts to wait for repotracker", "timed out waiting for repotracker to pick up new commits and create versions after %d attempts", repotrackerAttempts)
}

// submitSmokeTestPatch submits a manual patch to the app server to run the
// smoke test and selects tasks in the given build variant.
// Note that this requires using the CLI because there's currently no way to
// create a patch from the REST API.
func submitSmokeTestPatch(ctx context.Context, t *testing.T, params SmokeTestParams) {
	grip.Info("Submitting patch to smoke test app server.")

	cmd, err := internal.SmokeRunBinary(ctx, "smoke-patch-submission", params.EVGHome, params.CLIPath, "-c", params.CLIConfigPath, "patch", "-p", params.ProjectID, "-v", params.BVName, "-t", "all", "-f", "-y", "-d", "Smoke test patch")
	require.NoError(t, err, "should have submitted patch")
	require.NoError(t, cmd.Wait(), "expected to finish successful CLI patch")

	grip.Info("Successfully submitted patch to smoke test app server.")
}

// getSmokeTestPatch gets the user's manual patch that was submitted to the app
// server. It returns the patch ID.
func getSmokeTestPatch(ctx context.Context, t *testing.T, params SmokeTestParams, client *http.Client) string {
	grip.Infof("Waiting for manual patch for user '%s' to exist.", params.Username)

	const patchCheckAttempts = 10
	for i := 0; i < patchCheckAttempts; i++ {
		time.Sleep(2 * time.Second)
		respBody, err := internal.MakeSmokeRequest(ctx, params.APIParams, http.MethodGet, client, fmt.Sprintf("/rest/v2/users/%s/patches?limit=1", params.Username))
		if err != nil {
			grip.Error(errors.Wrapf(err, "requesting latest patches for user '%s'", params.Username))
			continue
		}
		if len(respBody) == 0 {
			grip.Error(errors.Errorf("did not find any latest patches yet for user '%s'", params.Username))
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
			grip.Error(errors.Errorf("listing latest patches for user '%s' yielded no results", params.Username))
			continue
		}

		latestPatch := latestPatches[0]
		latestPatchID := utility.FromStringPtr(latestPatch.Id)
		if createTime := utility.FromTimePtr(latestPatch.CreateTime); time.Since(createTime) > time.Hour {
			grip.Infof("Found latest patch '%s' in project '%s', but it was created at %s, waiting for patch that was just submitted", latestPatchID, utility.FromStringPtr(latestPatch.ProjectId), createTime)
			continue
		}

		grip.Infof("Successfully found patch '%s' in project '%s' submitted by user '%s'.", latestPatchID, params.ProjectID, params.Username)

		return latestPatchID
	}

	require.FailNow(t, "ran out of attempts to get smoke test patch", "timed out waiting patch to exist after %d attempts", patchCheckAttempts)

	return ""
}

// smokeAPIBuild represents part of a build from the REST API for use in the
// smoke test.
type smokeAPIBuild struct {
	Tasks []string `json:"tasks"`
}

// getAndCheckBuilds gets build and task information from the Evergreen app
// server's REST API for the smoke test's manual patch.
func getAndCheckBuilds(ctx context.Context, params SmokeTestParams, patchID string, client *http.Client) ([]smokeAPIBuild, error) {
	grip.Infof("Attempting to get builds created by the manual patch '%s'.", patchID)

	const buildCheckAttempts = 10
	for i := 0; i < buildCheckAttempts; i++ {
		// Poll the app server until the patch's builds and tasks exist.
		time.Sleep(2 * time.Second)

		grip.Infof("Checking for a build of patch '%s'. (%d/%d)", patchID, i+1, buildCheckAttempts)
		body, err := internal.MakeSmokeRequest(ctx, params.APIParams, http.MethodGet, client, fmt.Sprintf("/rest/v2/versions/%s/builds", patchID))
		if err != nil {
			grip.Error(errors.Wrap(err, "requesting builds"))
			continue
		}

		builds := []smokeAPIBuild{}
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
