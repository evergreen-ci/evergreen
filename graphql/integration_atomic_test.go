package graphql_test

// This test takes a specification and runs GraphQL queries, comparing the output of the query to what is expected.
// To add a new test:
// 1. Add a new directory in the tests directory. Name it after the query/mutation you are testing.
// 2. Add a data.json file to the dir you created. The data for your tests goes here. See tests/patchTasks/data.json for example.
// 3. (Optional) Add directory specific test setup within the directorySpecificTestSetup function.
// 4. (Optional) Add directory specific test cleanup within the directorySpecificTestCleanup function.
// 5. Add a results.json file to the dir you created. The results that your queries will be asserts against go here. See tests/patchTasks/results.json for example.
// 6. Create a queries dir in the dir you created. All the queries/mutations for your tests go in this dir.
// 7. That's all! Start testing.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type atomicGraphQLState struct {
	server      *service.TestServer
	apiUser     string
	apiKey      string
	directory   string
	taskLogDB   string
	taskLogColl string
	testData    map[string]json.RawMessage
	settings    *evergreen.Settings
}

func TestAtomicGQLQueries(t *testing.T) {
	grip.Warning(grip.SetSender(send.MakePlainLogger()))
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestAtomicGQLQueries")
	testDirectories, err := ioutil.ReadDir("tests")
	require.NoError(t, err)
	server, err := service.CreateTestServer(settings, nil, true)
	require.NoError(t, err)
	defer server.Close()

	for _, dir := range testDirectories {
		state := atomicGraphQLState{
			taskLogDB:   model.TaskLogDB,
			taskLogColl: model.TaskLogCollection,
			directory:   dir.Name(),
			settings:    settings,
			server:      server,
		}
		t.Run(state.directory, makeTestsInDirectory(t, &state))
	}
}

const apiUser = "testuser"

func setup(t *testing.T, state *atomicGraphQLState) {
	const apiKey = "testapikey"
	const slackUsername = "testslackuser"
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	pubKeys := []user.PubKey{
		{Name: "z", Key: "zKey", CreatedAt: time.Time{}},
		{Name: "c", Key: "cKey", CreatedAt: time.Time{}},
		{Name: "d", Key: "dKey", CreatedAt: time.Time{}},
		{Name: "a", Key: "aKey", CreatedAt: time.Time{}},
		{Name: "b", Key: "bKey", CreatedAt: time.Time{}},
	}
	systemRoles := []string{"unrestrictedTaskAccess", "modify_host", "modify_project_tasks", "superuser"}
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))

	usr, err := user.GetOrCreateUser(apiUser, apiUser, email, accessToken, refreshToken, []string{})
	require.NoError(t, err)

	for _, pk := range pubKeys {
		err = usr.AddPublicKey(pk.Name, pk.Key)
		require.NoError(t, err)
	}
	err = usr.UpdateSettings(user.UserSettings{Timezone: "America/New_York", SlackUsername: slackUsername})
	require.NoError(t, err)

	for _, role := range systemRoles {
		err = usr.AddRole(role)
		require.NoError(t, err)
	}

	require.NoError(t, usr.UpdateAPIKey(apiKey))
	// Create scope and role collection to avoid RoleManager from trying to create them in a collection https://jira.mongodb.org/browse/EVG-15499
	require.NoError(t, env.DB().CreateCollection(ctx, evergreen.ScopeCollection))
	require.NoError(t, env.DB().CreateCollection(ctx, evergreen.RoleCollection))

	require.NoError(t, setupData(*env.DB(), *env.Client().Database(state.taskLogDB), state.testData, *state))
	roleManager := env.RoleManager()

	roles, err := roleManager.GetAllRoles()
	require.NoError(t, err)
	require.Len(t, roles, 0)

	distroScope := gimlet.Scope{
		ID:        evergreen.AllDistrosScope,
		Name:      "modify host scope",
		Type:      evergreen.DistroResourceType,
		Resources: []string{"ubuntu1604-small", "ubuntu1604-large"},
	}
	err = roleManager.AddScope(distroScope)
	require.NoError(t, err)

	modifyHostRole := gimlet.Role{
		ID:          "modify_host",
		Name:        evergreen.HostsEdit.Description,
		Scope:       evergreen.AllDistrosScope,
		Permissions: map[string]int{evergreen.PermissionHosts: evergreen.HostsEdit.Value},
	}
	err = roleManager.UpdateRole(modifyHostRole)
	require.NoError(t, err)

	modifyProjectTasks := gimlet.Scope{
		ID:        "modify_tasks_scope",
		Name:      "modify tasks scope",
		Type:      evergreen.ProjectResourceType,
		Resources: []string{"spruce"},
	}
	err = roleManager.AddScope(modifyProjectTasks)
	require.NoError(t, err)

	modifyProjectTaskRole := gimlet.Role{
		ID:          "modify_project_tasks",
		Name:        evergreen.TasksAdmin.Description,
		Scope:       modifyProjectTasks.ID,
		Permissions: map[string]int{evergreen.PermissionTasks: evergreen.TasksAdmin.Value},
	}
	err = roleManager.UpdateRole(modifyProjectTaskRole)
	require.NoError(t, err)

	superUserRole := gimlet.Role{
		ID:          "superuser",
		Name:        "superuser",
		Scope:       "superuser_scope",
		Permissions: map[string]int{"admin_settings": 10, "project_create": 10, "distro_create": 10, "modify_roles": 10},
	}
	err = roleManager.UpdateRole(superUserRole)
	require.NoError(t, err)

	superUserScope := gimlet.Scope{
		ID:        "superuser_scope",
		Name:      "superuser scope",
		Type:      evergreen.SuperUserResourceType,
		Resources: []string{"super_user"},
	}
	err = roleManager.AddScope(superUserScope)
	require.NoError(t, err)

	state.apiKey = apiKey
	state.apiUser = apiUser

	directorySpecificTestSetup(t, *state)
}

type testsCases struct {
	Tests []test `json:"tests"`
}

func makeTestsInDirectory(t *testing.T, state *atomicGraphQLState) func(t *testing.T) {
	return func(t *testing.T) {
		dataFile, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "data.json"))
		require.NoError(t, err)

		resultsFile, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "results.json"))
		require.NoError(t, err)

		var testData map[string]json.RawMessage
		err = json.Unmarshal(dataFile, &testData)
		require.NoError(t, err)
		state.testData = testData

		var tests testsCases
		err = json.Unmarshal(resultsFile, &tests)
		require.NoError(t, err)

		// Delete exactly the documents added to the task_logg coll instead of dropping task log db
		// we do this to minimize deleting data that was not added from this test suite
		if testData[state.taskLogColl] != nil {
			logsDb := evergreen.GetEnvironment().Client().Database(state.taskLogDB)
			idArr := []string{}
			var docs []model.TaskLog
			require.NoError(t, bson.UnmarshalExtJSON(testData[state.taskLogColl], false, &docs))
			for _, d := range docs {
				idArr = append(idArr, d.Id)
			}
			_, err := logsDb.Collection(state.taskLogColl).DeleteMany(context.Background(), bson.M{"_id": bson.M{"$in": idArr}})
			require.NoError(t, err)
		}

		setup(t, state)
		for _, testCase := range tests.Tests {
			singleTest := func(t *testing.T) {
				f, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "queries", testCase.QueryFile))
				require.NoError(t, err)
				jsonQuery := fmt.Sprintf(`{"operationName":null,"variables":{},"query":"%s"}`, escapeGQLQuery(string(f)))
				body := bytes.NewBuffer([]byte(jsonQuery))
				client := http.Client{}
				r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql/query", state.server.URL), body)
				require.NoError(t, err)
				r.Header.Add(evergreen.APIKeyHeader, state.apiKey)
				r.Header.Add(evergreen.APIUserHeader, state.apiUser)
				r.Header.Add("content-type", "application/json")
				resp, err := client.Do(r)
				require.NoError(t, err)
				b, err := ioutil.ReadAll(resp.Body)
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
				additionalChecks(t)
			}

			t.Run(testCase.QueryFile, singleTest)
		}
		directorySpecificTestCleanup(t, state.directory)
	}
}

func setupData(db mongo.Database, logsDb mongo.Database, data map[string]json.RawMessage, state atomicGraphQLState) error {
	ctx := context.Background()
	catcher := grip.NewBasicCatcher()
	for coll, d := range data {
		var docs []interface{}
		// the docs to insert as part of setup need to be deserialized as extended JSON, whereas the rest of the
		// test spec is normal JSON
		catcher.Add(bson.UnmarshalExtJSON(d, false, &docs))
		// task_logg collection belongs to the logs db
		if coll == state.taskLogColl {
			_, err := logsDb.Collection(coll).InsertMany(ctx, docs)
			catcher.Add(err)
		} else {
			_, err := db.Collection(coll).InsertMany(ctx, docs)
			catcher.Add(err)
		}
	}
	return catcher.Resolve()
}

func directorySpecificTestSetup(t *testing.T, state atomicGraphQLState) {
	persistTestSettings := func(t *testing.T) {
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": build.Collection})
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": task.Collection})
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": model.VersionCollection})
		_ = evergreen.GetEnvironment().DB().RunCommand(nil, map[string]string{"create": model.ParserProjectCollection})
		require.NoError(t, state.settings.Set())

	}
	type setupFn func(*testing.T)
	// Map the directory name to the test setup function
	m := map[string][]setupFn{
		"attachVolumeToHost":   {spawnTestHostAndVolume},
		"detachVolumeFromHost": {spawnTestHostAndVolume},
		"removeVolume":         {spawnTestHostAndVolume},
		"spawnVolume":          {spawnTestHostAndVolume, addSubnets},
		"updateVolume":         {spawnTestHostAndVolume},
		"schedulePatch":        {persistTestSettings},
	}
	if m[state.directory] != nil {
		for _, exec := range m[state.directory] {
			exec(t)
		}
	}
}

func directorySpecificTestCleanup(t *testing.T, directory string) {
	type cleanupFn func(*testing.T)
	// Map the directory name to the test cleanup function
	m := map[string][]cleanupFn{
		"spawnVolume": {clearSubnets},
	}
	if m[directory] != nil {
		for _, exec := range m[directory] {
			exec(t)
		}
	}
}

func spawnTestHostAndVolume(t *testing.T) {
	// Initialize Spawn Host and Spawn Volume used in tests
	volExp, err := time.Parse(time.RFC3339, "2020-06-06T14:43:06.287Z")
	require.NoError(t, err)
	volCreation, err := time.Parse(time.RFC3339, "2020-06-05T14:43:06.567Z")
	require.NoError(t, err)
	mountedVolume := host.Volume{
		ID:               "vol-0603934da6f024db5",
		DisplayName:      "cd372fb85148700fa88095e3492d3f9f5beb43e555e5ff26d95f5a6adc36f8e6",
		CreatedBy:        apiUser,
		Type:             "6937b1605cf6131b7313c515fb4cd6a3b27605ba318c9d6424584499bc312c0b",
		Size:             500,
		AvailabilityZone: "us-east-1a",
		Expiration:       volExp,
		NoExpiration:     false,
		CreationDate:     volCreation,
		Host:             "i-1104943f",
		HomeVolume:       true,
	}
	require.NoError(t, mountedVolume.Insert())
	h := host.Host{
		Id:     "i-1104943f",
		Host:   "i-1104943f",
		User:   apiUser,
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
		Project:            "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		Zone:               "us-east-1a",
		Provisioned:        true,
	}
	require.NoError(t, h.Insert())
	ctx := context.Background()
	err = graphql.SpawnHostForTestCode(ctx, &mountedVolume, &h)
	require.NoError(t, err)
}

func addSubnets(t *testing.T) {
	evergreen.GetEnvironment().Settings().Providers.AWS.Subnets = []evergreen.Subnet{{AZ: "us-east-1a", SubnetID: "new_id"}}
}

func clearSubnets(t *testing.T) {
	evergreen.GetEnvironment().Settings().Providers.AWS.Subnets = []evergreen.Subnet{}
}

func additionalChecks(t *testing.T) {
	var checks = map[string]func(*testing.T){
		// note these 2 are only the same because the same project ID is used
		"TestAtomicGQLQueries/abortTask/commit-queue-dequeue.graphql":            checkCommitQueueDequeued,
		"TestAtomicGQLQueries/unschedulePatchTasks/commit-queue-dequeue.graphql": checkCommitQueueDequeued,
	}
	if check, exists := checks[t.Name()]; exists {
		check(t)
	}
}

func checkCommitQueueDequeued(t *testing.T) {
	cq, err := commitqueue.FindOneId("p1")
	assert.NoError(t, err)
	assert.Empty(t, cq.Queue)
}
