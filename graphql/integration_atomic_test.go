package graphql_test

// This test takes a specification and runs GraphQL queries, comparing the output of the query to what is expected.
// To add a new test:
// 1. Add a new directory in the tests directory. Name it after the query/mutation you are testing.
// 2. Add a data.json file to the dir you created. The data for your tests goes here. See tests/patchTasks/data.json for example.
// 3. (Optional) Add directory specific test setup within the directorySpecificTestSetup function.
// 4. Add a results.json file to the dir you created. The results that your queries will be asserts against go here. See tests/patchTasks/results.json for example.
// 5. Create a queries dir in the dir you created. All the queries/mutations for your tests go in this dir.
// 6. That's all! Start testing.

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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type atomicGraphQLState struct {
	url         string
	apiUser     string
	apiKey      string
	directory   string
	taskLogDB   string
	taskLogColl string
}

func TestAtomicGQLQueries(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, evergreen.GetEnvironment().Settings(), "TestAtomicGQLQueries")
	testDirectories, err := ioutil.ReadDir("tests")
	require.NoError(t, err)
	for _, dir := range testDirectories {
		state := setup(t, dir.Name())
		runTestsInDirectory(t, state)
	}
}

func setup(t *testing.T, directory string) atomicGraphQLState {
	const apiKey = "testapikey"
	const apiUser = "testuser"
	const slackUsername = "testslackuser"
	state := atomicGraphQLState{taskLogDB: model.TaskLogDB, taskLogColl: model.TaskLogCollection}
	server, err := service.CreateTestServer(testutil.TestConfig(), nil, true)
	require.NoError(t, err)
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))
	testUser := user.DBUser{
		Id:          apiUser,
		APIKey:      apiKey,
		Settings:    user.UserSettings{Timezone: "America/New_York", SlackUsername: slackUsername},
		SystemRoles: []string{"unrestrictedTaskAccess", "modify_host"},
		PubKeys: []user.PubKey{
			user.PubKey{Name: "z", Key: "zKey", CreatedAt: time.Time{}},
			user.PubKey{Name: "c", Key: "cKey", CreatedAt: time.Time{}},
			user.PubKey{Name: "d", Key: "dKey", CreatedAt: time.Time{}},
			user.PubKey{Name: "a", Key: "aKey", CreatedAt: time.Time{}},
			user.PubKey{Name: "b", Key: "bKey", CreatedAt: time.Time{}},
		}}
	require.NoError(t, testUser.Insert())
	modifyHostRole := gimlet.Role{
		ID:          "modify_host",
		Name:        "modify host",
		Scope:       "modify_host_scope",
		Permissions: map[string]int{"distro_hosts": 20},
	}
	_, err = env.DB().Collection("roles").InsertOne(ctx, modifyHostRole)
	require.NoError(t, err)

	modifyHostScope := gimlet.Scope{
		ID:        "modify_host_scope",
		Name:      "modify host scope",
		Type:      "distro",
		Resources: []string{"ubuntu1604-small", "ubuntu1604-large"},
	}
	_, err = env.DB().Collection("scopes").InsertOne(ctx, modifyHostScope)
	require.NoError(t, err)
	directorySpecificTestSetup(t, directory)
	state.url = server.URL
	state.apiKey = apiKey
	state.apiUser = apiUser
	state.directory = directory

	return state
}

type testsCases struct {
	Tests []test `json:"tests"`
}

func runTestsInDirectory(t *testing.T, state atomicGraphQLState) {
	dataFile, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "data.json"))
	require.NoError(t, err)

	resultsFile, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "results.json"))
	require.NoError(t, err)

	var testData map[string]json.RawMessage
	err = json.Unmarshal(dataFile, &testData)
	require.NoError(t, err)

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

	require.NoError(t, setupData(*evergreen.GetEnvironment().DB(), *evergreen.GetEnvironment().Client().Database(state.taskLogDB), testData, state))

	for _, testCase := range tests.Tests {
		singleTest := func(t *testing.T) {
			f, err := ioutil.ReadFile(filepath.Join("tests", state.directory, "queries", testCase.QueryFile))
			require.NoError(t, err)
			jsonQuery := fmt.Sprintf(`{"operationName":null,"variables":{},"query":"%s"}`, escapeGQLQuery(string(f)))
			body := bytes.NewBuffer([]byte(jsonQuery))
			client := http.Client{}
			r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql/query", state.url), body)
			require.NoError(t, err)
			r.Header.Add(evergreen.APIKeyHeader, state.apiKey)
			r.Header.Add(evergreen.APIUserHeader, state.apiUser)
			r.Header.Add("content-type", "application/json")
			resp, err := client.Do(r)
			require.NoError(t, err)
			b, err := ioutil.ReadAll(resp.Body)
			require.NoError(t, err)
			assert.JSONEq(t, string(testCase.Result), string(b), fmt.Sprintf("expected %s but got %s", string(testCase.Result), string(b)))
		}

		t.Run(fmt.Sprintf("%s/%s", state.directory, testCase.QueryFile), singleTest)
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

func directorySpecificTestSetup(t *testing.T, directory string) {
	type setupFn func(*testing.T)
	// Map the directory name to the test setup function
	m := map[string]setupFn{
		"attachVolumeToHost":   spawnTestHostAndVolume,
		"detachVolumeFromHost": spawnTestHostAndVolume,
		"removeVolume":         spawnTestHostAndVolume,
	}
	if m[directory] != nil {
		m[directory](t)
	}
}

func spawnTestHostAndVolume(t *testing.T) {
	// Initialize Spawn Host and Spawn Volume used in tests
	volExp, err := time.Parse(time.RFC3339, "2020-06-06T14:43:06.287Z")
	require.NoError(t, err)
	volCreation, err := time.Parse(time.RFC3339, "2020-06-05T14:43:06.567Z")
	require.NoError(t, err)
	volume := host.Volume{
		ID:               "vol-0603934da6f024db5",
		DisplayName:      "cd372fb85148700fa88095e3492d3f9f5beb43e555e5ff26d95f5a6adc36f8e6",
		CreatedBy:        "ae5deb822e0d71992900471a7199d0d95b8e7c9d05c40a8245a281fd2c1d6684",
		Type:             "6937b1605cf6131b7313c515fb4cd6a3b27605ba318c9d6424584499bc312c0b",
		Size:             500,
		AvailabilityZone: "us-east-1a",
		Expiration:       volExp,
		NoExpiration:     false,
		CreationDate:     volCreation,
		Host:             "i-1104943f",
		HomeVolume:       true,
	}
	require.NoError(t, volume.Insert())
	h := host.Host{
		Id:     "i-1104943f",
		Host:   "i-1104943f",
		User:   "b17ff2bce48644cfd2f8c8b9ea72c6a302f617273f56be515b3db0df0c76cb5b",
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
		ProvisionAttempts:  0,
	}
	require.NoError(t, h.Insert())
	ctx := context.Background()
	err = graphql.SpawnHostForTestCode(ctx, &volume, &h)
	require.NoError(t, err)
}
