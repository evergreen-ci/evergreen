package graphql

// This test takes a specification and runs GraphQL queries, comparing the output of the query to what is expected.
// To add a new test:
// 1. Add a new directory in the tests directory. Name it after the query/mutation you are testing.
// 2. Add a data.json file to the dir you created. The data for your tests goes here. See tests/versionTasks/data.json for example.
// 3. (Optional) Add a task_output_data.json file to the dir you created. The "offline" (not stored in the DB) task output data, such as task and test logs, goes here. See tests/task/taskLogs/task_output_data.json for example.
// 4. (Optional) Add directory specific test setup within the directorySpecificTestSetup function.
// 5. (Optional) Add directory specific test cleanup within the directorySpecificTestCleanup function.
// 6. Add a results.json file to the dir you created. The results that your queries will be asserts against go here. See tests/versionTasks/results.json for example.
// 7. Create a queries dir in the dir you created. All the queries/mutations for your tests go in this dir.
// 8. That's all! Start testing.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	// Use this for GraphQL tests that are written in Golang (e.g. directive_test.go).
	testUser = "test_user"

	adminUser      = "admin_user"
	privilegedUser = "privileged_user"
	regularUser    = "regular_user"
)

type AtomicGraphQLState struct {
	ServerURL      string
	Directory      string
	DBData         map[string]json.RawMessage
	TaskOutputData map[string]json.RawMessage
	Settings       *evergreen.Settings
}

func MakeTestsInDirectory(state *AtomicGraphQLState, pathToTests string) func(t *testing.T) {
	return func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		dbDataFilePath := filepath.Join(pathToTests, "tests", state.Directory, "data.json")
		data, err := os.ReadFile(dbDataFilePath)
		require.NoError(t, err, "reading DB data file '%s'", dbDataFilePath)
		var dbData map[string]json.RawMessage
		require.NoError(t, json.Unmarshal(data, &dbData), "unmarshalling data file for '%s'", dbDataFilePath)
		state.DBData = dbData

		taskOutputDataFilePath := filepath.Join(pathToTests, "tests", state.Directory, "task_output_data.json")
		if _, err = os.Stat(taskOutputDataFilePath); !os.IsNotExist(err) {
			data, err = os.ReadFile(taskOutputDataFilePath)
			require.NoError(t, err, "reading task output file '%s'", taskOutputDataFilePath)
			var taskOutputData map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(data, &taskOutputData), "unmarshalling task output file for '%s'", taskOutputDataFilePath)
			state.TaskOutputData = taskOutputData
		}

		resultsFilePath := filepath.Join(pathToTests, "tests", state.Directory, "results.json")
		data, err = os.ReadFile(resultsFilePath)
		require.NoError(t, err, "reading results file '%s'", resultsFilePath)
		var tests testsCases
		err = json.Unmarshal(data, &tests)
		require.NoError(t, errors.Wrapf(err, "unmarshalling results file for %s", resultsFilePath))

		// Reset database and populate with current test suite's data.
		env := evergreen.GetEnvironment()
		require.NoError(t, env.DB().Drop(ctx))
		require.NoError(t, setupDBIndexes())
		require.NoError(t, setupDBData(ctx, env, state.DBData))
		require.NoError(t, setupTaskOutputData(ctx, state))
		setupUsers(t)
		setupScopesAndRoles(t, state)

		for _, testCase := range tests.Tests {
			singleTest := func(t *testing.T) {
				f, err := os.ReadFile(filepath.Join(pathToTests, "tests", state.Directory, "queries", testCase.QueryFile))
				require.NoError(t, err)
				jsonQuery := fmt.Sprintf(`{"operationName":null,"variables":{},"query":"%s"}`, escapeGQLQuery(string(f)))
				body := bytes.NewBuffer([]byte(jsonQuery))
				client := http.Client{}
				r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql/query", state.ServerURL), body)
				require.NoError(t, err)

				testUserId := utility.FromStringPtr(testCase.TestUserId)
				// Default to the admin user if no user is specified to run the test as.
				if testUserId == "" {
					testUserId = adminUser
				}
				foundUser, err := user.FindOneByIdContext(t.Context(), testUserId)
				require.NoError(t, err)
				r.Header.Add(evergreen.APIUserHeader, foundUser.Id)
				r.Header.Add(evergreen.APIKeyHeader, foundUser.APIKey)

				r.Header.Add("content-type", "application/json")
				resp, err := client.Do(r)
				require.NoError(t, err)
				defer resp.Body.Close()
				b, err := io.ReadAll(resp.Body)
				require.NoError(t, err)

				// Remove apollo tracing data from test responses
				var bJSON map[string]json.RawMessage
				err = json.Unmarshal(b, &bJSON)
				require.NoError(t, err)

				delete(bJSON, "extensions")
				b, err = json.Marshal(bJSON)
				require.NoError(t, err)

				pass := assert.JSONEq(t, string(testCase.Result), string(b), "test failure, more details below (whitespace will not line up)")
				if !pass {
					var actual bytes.Buffer
					err = json.Indent(&actual, b, "", "  ")
					if err != nil {
						grip.Error(errors.Wrap(err, "actual value was not json"))
						return
					}
					grip.Info("=== expected ===")
					grip.Info(string(testCase.Result))
					grip.Info("=== actual ===")
					grip.Info(actual.Bytes())
				}
			}

			t.Run(testCase.QueryFile, singleTest)
		}
		directorySpecificTestCleanup(t, state.Directory)
	}
}

func setupUsers(t *testing.T) {
	const accessToken = "access_token"
	const refreshToken = "refresh_token"

	// Admin user is a superuser with admin project and distro access.
	adminUsr := user.DBUser{
		Id:           adminUser,
		DispName:     "Admin User",
		EmailAddress: "admin_user@mongodb.com",
		Settings: user.UserSettings{
			SlackUsername: "admin_slackuser",
			SlackMemberId: "12345member",
			UseSpruceOptions: user.UseSpruceOptions{
				SpruceV1: true,
			},
			Timezone: "America/New_York",
		},
		LoginCache: user.LoginCache{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		},
		APIKey: "admin_api_key",
		PubKeys: []user.PubKey{
			{Name: "z", Key: "zKey", CreatedAt: time.Time{}},
			{Name: "c", Key: "cKey", CreatedAt: time.Time{}},
			{Name: "d", Key: "dKey", CreatedAt: time.Time{}},
			{Name: "a", Key: "aKey", CreatedAt: time.Time{}},
			{Name: "b", Key: "bKey", CreatedAt: time.Time{}},
		},
		SystemRoles: []string{
			evergreen.SuperUserRole,
			evergreen.SuperUserProjectAccessRole,
			evergreen.SuperUserDistroAccessRole,
			"project_grumpyCat",
			"project_spruce",
			"project_sandbox",
			"project_evergreen",
			"repo_sandbox",
		},
	}
	assert.NoError(t, adminUsr.Insert(t.Context()))

	// Privileged user has admin project and distro access, but is not a superuser.
	privilegedUsr := user.DBUser{
		Id:           privilegedUser,
		DispName:     "Privileged User",
		EmailAddress: "privileged_user@mongodb.com",
		Settings: user.UserSettings{
			SlackUsername: "privileged_slackuser",
			SlackMemberId: "12345member",
			UseSpruceOptions: user.UseSpruceOptions{
				SpruceV1: true,
			},
			Timezone: "America/New_York",
		},
		LoginCache: user.LoginCache{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		},
		APIKey: "privileged_api_key",
		SystemRoles: []string{
			evergreen.SuperUserProjectAccessRole,
			evergreen.SuperUserDistroAccessRole,
			"project_happyAbyssinian",
		},
	}
	assert.NoError(t, privilegedUsr.Insert(t.Context()))

	// Regular user only has basic project and distro access.
	regularUser := user.DBUser{
		Id:           regularUser,
		DispName:     "Regular User",
		EmailAddress: "regular_user@mongodb.com",
		Settings: user.UserSettings{
			SlackUsername: "regular_slackuser",
			SlackMemberId: "12345member",
			UseSpruceOptions: user.UseSpruceOptions{
				SpruceV1: true,
			},
			Timezone: "America/New_York",
		},
		LoginCache: user.LoginCache{
			AccessToken:  accessToken,
			RefreshToken: refreshToken,
		},
		APIKey: "regular_api_key",
		SystemRoles: []string{
			evergreen.BasicProjectAccessRole,
			evergreen.BasicDistroAccessRole,
		},
	}
	assert.NoError(t, regularUser.Insert(t.Context()))
}

func setupScopesAndRoles(t *testing.T, state *AtomicGraphQLState) {
	env := evergreen.GetEnvironment()
	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles()
	require.NoError(t, err)
	require.Empty(t, roles)

	// Set up scopes and roles for projects.
	allProjectScope := gimlet.Scope{
		ID:        evergreen.AllProjectsScope,
		Name:      "all projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"mci", "happyAbyssinian", "grumpyCat", "spruce", "evergreen", "repo_sandbox", "project_sandbox"},
	}
	err = roleManager.AddScope(allProjectScope)
	require.NoError(t, err)

	unrestrictedProjectScope := gimlet.Scope{
		ID:        evergreen.UnrestrictedProjectsScope,
		Name:      "unrestricted projects",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"spruce"},
	}
	err = roleManager.AddScope(unrestrictedProjectScope)
	require.NoError(t, err)

	adminProjectAccessRole := gimlet.Role{
		ID:    evergreen.SuperUserProjectAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllProjectsScope,
		Permissions: map[string]int{
			evergreen.PermissionProjectSettings: evergreen.ProjectSettingsView.Value,
			evergreen.PermissionTasks:           evergreen.TasksBasic.Value,
			evergreen.PermissionPatches:         evergreen.PatchNone.Value,
			evergreen.PermissionLogs:            evergreen.LogsView.Value,
			evergreen.PermissionAnnotations:     evergreen.AnnotationsModify.Value,
		},
	}
	err = roleManager.UpdateRole(adminProjectAccessRole)
	require.NoError(t, err)

	basicProjectAccessRole := gimlet.Role{
		ID:    evergreen.BasicProjectAccessRole,
		Name:  "basic access",
		Scope: evergreen.UnrestrictedProjectsScope,
		Permissions: map[string]int{
			evergreen.PermissionTasks:       evergreen.TasksBasic.Value,
			evergreen.PermissionPatches:     evergreen.PatchNone.Value,
			evergreen.PermissionLogs:        evergreen.LogsView.Value,
			evergreen.PermissionAnnotations: evergreen.AnnotationsModify.Value,
		},
	}
	err = roleManager.UpdateRole(basicProjectAccessRole)
	require.NoError(t, err)

	// Set up scopes and roles for distros.
	distroScope := gimlet.Scope{
		ID:        evergreen.AllDistrosScope,
		Name:      "all distros",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"ubuntu1604-small", "ubuntu1604-large", "localhost", "localhost2", "rhel71-power8-large", "windows-64-vs2015-small", "ubuntu1604-power8-large", "centos6-perf", "macos-1014", "debian92-small", "ubuntu1804-power8-small"},
	}
	err = roleManager.AddScope(distroScope)
	require.NoError(t, err)

	adminDistroAccessRole := gimlet.Role{
		ID:    evergreen.SuperUserDistroAccessRole,
		Name:  "admin access",
		Scope: evergreen.AllDistrosScope,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsAdmin.Value,
			evergreen.PermissionHosts:          evergreen.HostsEdit.Value,
		},
	}
	require.NoError(t, roleManager.UpdateRole(adminDistroAccessRole))

	basicDistroAccessRole := gimlet.Role{
		ID:    evergreen.BasicDistroAccessRole,
		Name:  "basic access",
		Scope: evergreen.AllDistrosScope,
		Permissions: map[string]int{
			evergreen.PermissionDistroSettings: evergreen.DistroSettingsView.Value,
			evergreen.PermissionHosts:          evergreen.HostsView.Value,
		},
	}
	err = roleManager.UpdateRole(basicDistroAccessRole)
	require.NoError(t, err)

	// Set up scopes and roles for superuser.
	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{evergreen.SuperUserPermissionsID},
	}
	err = roleManager.AddScope(superUserScope)
	require.NoError(t, err)

	superUserRole := gimlet.Role{
		ID:    evergreen.SuperUserRole,
		Name:  "superuser",
		Scope: superUserScope.ID,
		Permissions: map[string]int{
			evergreen.PermissionAdminSettings: evergreen.AdminSettingsEdit.Value,
			evergreen.PermissionProjectCreate: evergreen.ProjectCreate.Value,
			evergreen.PermissionDistroCreate:  evergreen.DistroCreate.Value,
			evergreen.PermissionRoleModify:    evergreen.RoleModify.Value,
		},
	}
	err = roleManager.UpdateRole(superUserRole)
	require.NoError(t, err)

	// Set up scopes and roles for individual projects.
	projectPermissions := map[string]int{
		evergreen.PermissionProjectSettings: evergreen.ProjectSettingsEdit.Value,
		evergreen.PermissionAnnotations:     evergreen.AnnotationsModify.Value,
		evergreen.PermissionTasks:           evergreen.TasksAdmin.Value,
		evergreen.PermissionPatches:         evergreen.PatchSubmitAdmin.Value,
		evergreen.PermissionLogs:            evergreen.LogsView.Value,
	}

	projectSpruceScope := gimlet.Scope{
		ID:        "project_spruce_scope",
		Name:      "spruce",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"spruce"},
	}
	err = roleManager.AddScope(projectSpruceScope)
	require.NoError(t, err)

	projectSpruceRole := gimlet.Role{
		ID:          "project_spruce",
		Name:        "spruce",
		Scope:       projectSpruceScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(projectSpruceRole)
	require.NoError(t, err)

	projectEvergreenScope := gimlet.Scope{
		ID:        "project_evergreen_scope",
		Name:      "evergreen",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"evergreen"},
	}
	err = roleManager.AddScope(projectEvergreenScope)
	require.NoError(t, err)

	projectEvergreenRole := gimlet.Role{
		ID:          "project_evergreen",
		Name:        "evergreen",
		Scope:       projectEvergreenScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(projectEvergreenRole)
	require.NoError(t, err)

	projectSandboxScope := gimlet.Scope{
		ID:        "project_sandbox_scope",
		Name:      "sandbox",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"sandbox_project_id"},
	}
	err = roleManager.AddScope(projectSandboxScope)
	require.NoError(t, err)

	projectSandboxRole := gimlet.Role{
		ID:          "project_sandbox",
		Name:        "sandbox",
		Scope:       projectSandboxScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(projectSandboxRole)
	require.NoError(t, err)

	projectGrumpyCatScope := gimlet.Scope{
		ID:        "project_grumpyCat_scope",
		Name:      "grumpyCat",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"grumpyCat"},
	}
	err = roleManager.AddScope(projectGrumpyCatScope)
	require.NoError(t, err)

	projectGrumpyCatRole := gimlet.Role{
		ID:          "project_grumpyCat",
		Name:        "grumpyCat",
		Scope:       projectGrumpyCatScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(projectGrumpyCatRole)
	require.NoError(t, err)

	projectHappyAbyssinianScope := gimlet.Scope{
		ID:        "project_happyAbyssinian_scope",
		Name:      "happyAbyssinian",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"happyAbyssinian"},
	}
	err = roleManager.AddScope(projectHappyAbyssinianScope)
	require.NoError(t, err)

	projectHappyAbyssinianRole := gimlet.Role{
		ID:          "project_happyAbyssinian",
		Name:        "happyAbyssinian",
		Scope:       projectHappyAbyssinianScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(projectHappyAbyssinianRole)
	require.NoError(t, err)

	// Set up scopes and roles for individual repos.
	repoSandboxScope := gimlet.Scope{
		ID:        "repo_sandbox_scope",
		Name:      "repo_sandbox",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"sandbox_repo_id"},
	}
	err = roleManager.AddScope(repoSandboxScope)
	require.NoError(t, err)

	repoSandboxRole := gimlet.Role{
		ID:          "repo_sandbox",
		Name:        "repo_sandbox",
		Scope:       repoSandboxScope.ID,
		Permissions: projectPermissions,
	}
	err = roleManager.UpdateRole(repoSandboxRole)
	require.NoError(t, err)

	directorySpecificTestSetup(t, *state)
}

type testsCases struct {
	Tests []test `json:"tests"`
}

type test struct {
	QueryFile  string          `json:"query_file"`
	TestUserId *string         `json:"test_user_id"`
	Result     json.RawMessage `json:"result"`
}

// escapeGQLQuery replaces literal newlines with '\n' and literal double quotes with '\"'
func escapeGQLQuery(in string) string {
	return strings.Replace(strings.Replace(in, "\n", "\\n", -1), "\"", "\\\"", -1)
}

// setupDBIndexes ensures that the indexes required for the tests are created.
func setupDBIndexes() error {
	if err := db.EnsureIndex(host.Collection, mongo.IndexModel{Keys: host.DistroIdStatusIndex}); err != nil {
		return errors.Wrap(err, "setting up host collection indexes")
	}
	if err := db.EnsureIndex(task.Collection, mongo.IndexModel{Keys: model.TaskHistoryIndex}); err != nil {
		return errors.Wrap(err, "setting up task collection indexes")
	}
	return nil
}

func setupDBData(ctx context.Context, env evergreen.Environment, data map[string]json.RawMessage) error {
	catcher := grip.NewBasicCatcher()

	for coll, d := range data {
		// The docs to insert as part of setup need to be deserialized
		// as extended JSON, whereas the rest of the test spec is
		// normal JSON.
		var docs []any
		catcher.Add(bson.UnmarshalExtJSON(d, false, &docs))
		_, err := env.DB().Collection(coll).InsertMany(ctx, docs)
		catcher.Add(err)
	}

	return catcher.Resolve()
}

func setupTaskOutputData(ctx context.Context, state *AtomicGraphQLState) error {
	for taskOutputType, data := range state.TaskOutputData {
		switch taskOutputType {
		case task.TaskLogOutput{}.ID():
			if err := setupTaskLogData(ctx, data); err != nil {
				return errors.Wrap(err, "setting up task log data")
			}
		default:
			return errors.Errorf("unsupported task output type '%s'", taskOutputType)
		}
	}

	return nil
}

func setupTaskLogData(ctx context.Context, data json.RawMessage) error {
	taskLogs := []struct {
		TaskID    string           `json:"task_id"`
		Execution int              `json:"execution"`
		LogType   task.TaskLogType `json:"log_type"`
		Lines     []log.LogLine    `json:"lines"`
	}{}
	if err := json.Unmarshal(data, &taskLogs); err != nil {
		return errors.Wrap(err, "unmarshalling task log data")
	}

	for _, taskLog := range taskLogs {
		tsk, err := task.FindByIdExecution(ctx, taskLog.TaskID, utility.ToIntPtr(taskLog.Execution))
		if err != nil {
			return errors.Wrap(err, "finding task for task log")
		}

		if tsk.TaskOutputInfo == nil {
			return errors.New("task missing task output info")
		}

		if err := task.AppendTaskLogs(ctx, *tsk, taskLog.LogType, taskLog.Lines); err != nil {
			return errors.Wrap(err, "appending task log lines")
		}
	}

	return nil
}

func directorySpecificTestSetup(t *testing.T, state AtomicGraphQLState) {
	persistTestSettings := func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": build.Collection})
		_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": task.Collection})
		_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": model.VersionCollection})
		_ = evergreen.GetEnvironment().DB().RunCommand(ctx, map[string]string{"create": model.ParserProjectCollection})
		require.NoError(t, state.Settings.Set(ctx))

	}

	setupAdminSettings := func(t *testing.T) {
		setupAdminSettingsFromData(t, state)
	}

	type setupFn func(*testing.T)
	// Map the directory name to the test setup function
	m := map[string][]setupFn{
		"mutation/attachVolumeToHost":   {spawnTestHostAndVolume},
		"mutation/detachVolumeFromHost": {spawnTestHostAndVolume},
		"mutation/removeVolume":         {spawnTestHostAndVolume},
		"mutation/spawnVolume":          {spawnTestHostAndVolume, addSubnets},
		"mutation/updateVolume":         {spawnTestHostAndVolume},
		"mutation/schedulePatch":        {persistTestSettings},
		"query/adminSettings":           {setupAdminSettings},
	}
	if m[state.Directory] != nil {
		for _, exec := range m[state.Directory] {
			exec(t)
		}
	}
}

func directorySpecificTestCleanup(t *testing.T, directory string) {
	type cleanupFn func(*testing.T)
	// Map the directory name to the test cleanup function
	m := map[string][]cleanupFn{
		"mutation/spawnVolume": {clearSubnets},
	}
	if m[directory] != nil {
		for _, exec := range m[directory] {
			exec(t)
		}
	}
}

func spawnTestHostAndVolume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Spawn Host and Spawn Volume used in tests
	volExp, err := time.Parse(time.RFC3339, "2020-06-06T14:43:06.287Z")
	require.NoError(t, err)
	volCreation, err := time.Parse(time.RFC3339, "2020-06-05T14:43:06.567Z")
	require.NoError(t, err)
	mountedVolume := host.Volume{
		ID:               "vol-0603934da6f024db5",
		DisplayName:      "cd372fb85148700fa88095e3492d3f9f5beb43e555e5ff26d95f5a6adc36f8e6",
		CreatedBy:        adminUser,
		Type:             "6937b1605cf6131b7313c515fb4cd6a3b27605ba318c9d6424584499bc312c0b",
		Size:             500,
		AvailabilityZone: "us-east-1a",
		Expiration:       volExp,
		NoExpiration:     false,
		CreationDate:     volCreation,
		Host:             "i-1104943f",
		HomeVolume:       true,
	}
	require.NoError(t, mountedVolume.Insert(t.Context()))
	h := host.Host{
		Id:     "i-1104943f",
		Host:   "i-1104943f",
		User:   adminUser,
		Secret: "",
		Tag:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Distro: distro.Distro{
			Id: "i-1104943f",
			Aliases: []string{
				"3a8d3c19862652b84e37111bc20e16d561d78902b5478f9170d7af6796ce40a3",
				"9ec394433d2dd99f422f21ceb50f62edcfba50255b84f1a274bf85295af26f09",
			},
			Arch:     "193b9ef5dfc4685c536b57c58c8d199b1eb1592dcd0ff3bea28af79d303c528d",
			WorkDir:  "b560622207b8a0d6354080f8363aa7d8a32c30e5d3309099a820217d0e7dc748",
			Provider: "2053dbbf6ec7135c4e994d3464c478db6f48d3ca21052c8f44915edc96e02c39",
			User:     "b17ff2bce48644cfd2f8c8b9ea72c6a302f617273f56be515b3db0df0c76cb5b",
		},
		Provider:           "2053dbbf6ec7135c4e994d3464c478db6f48d3ca21052c8f44915edc96e02c39",
		IP:                 "",
		ExternalIdentifier: "",
		DisplayName:        "",
		Zone:               "us-east-1a",
		Provisioned:        true,
	}
	require.NoError(t, h.Insert(ctx))
	err = spawnHostForTestCode(ctx, &mountedVolume, &h)
	require.NoError(t, err)
}

func spawnHostForTestCode(ctx context.Context, vol *host.Volume, h *host.Host) error {
	mgr, err := cloud.GetEC2ManagerForVolume(ctx, vol)
	if err != nil {
		return err
	}
	if os.Getenv(evergreen.SettingsOverride) != "" {
		// The mock manager needs to spawn the host specified in our test data.
		// The host should already be spawned in a non-test scenario.
		_, err := mgr.SpawnHost(ctx, h)
		if err != nil {
			return errors.Wrapf(err, "error spawning host in test code")
		}
	}
	return nil
}

func addSubnets(t *testing.T) {
	evergreen.GetEnvironment().Settings().Providers.AWS.Subnets = []evergreen.Subnet{{AZ: "us-east-1a", SubnetID: "new_id"}}
}

func clearSubnets(t *testing.T) {
	evergreen.GetEnvironment().Settings().Providers.AWS.Subnets = []evergreen.Subnet{}
}

func setupAdminSettingsFromData(t *testing.T, state AtomicGraphQLState) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get the admin data from the loaded test data
	adminRaw, exists := state.DBData["admin"]
	require.True(t, exists, "no admin data found in test data for adminSettings test")

	var adminData []interface{}
	err := bson.UnmarshalExtJSON(adminRaw, false, &adminData)
	require.NoError(t, err, "unmarshalling admin data")
	require.Len(t, adminData, 1, "expected exactly one admin document")

	adminDoc, ok := adminData[0].(map[string]interface{})
	require.True(t, ok, "admin data is not a map")

	bsonRaw, err := bson.Marshal(adminDoc)
	require.NoError(t, err, "marshalling admin data")

	var testSettings evergreen.Settings
	err = bson.Unmarshal(bsonRaw, &testSettings)
	require.NoError(t, err, "unmarshalling admin data into settings")

	restSettings := restModel.NewConfigModel()
	err = restSettings.BuildFromService(testSettings)
	require.NoError(t, err, "building API model from existing settings")

	u := &user.DBUser{Id: "user"}
	_, err = data.SetEvergreenSettings(ctx, restSettings, state.Settings, u, true)
	require.NoError(t, err, "setting admin settings")
}
