package graphql_test

// This test takes a specification and runs GraphQL queries, comparing the output of the query to what is expected.
// To add a new test:
// 1. Add a new directory in the tests directory. Name it after the query/mutation you are testing.
// 2. Add a data.json file to the dir you created. The data for your tests goes here. See tests/patchTasks/data.json for example.
// 3. Add a results.json file to the dir you created. The results that your queries will be asserts against go here. See tests/patchTasks/results.json for example.
// 4. Create a queries dir in the dir you created. All the queries/mutations for your tests go in this dir.
// 5. That's all! Start testing.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type atomicGraphQLState struct {
	url       string
	apiUser   string
	apiKey    string
	directory string
}

func TestAtomicGQLQueries(t *testing.T) {
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

	state := atomicGraphQLState{}
	server, err := service.CreateTestServer(testutil.TestConfig(), nil, true)
	require.NoError(t, err)
	env := evergreen.GetEnvironment()
	ctx := context.Background()
	require.NoError(t, env.DB().Drop(ctx))
	testUser := user.DBUser{
		Id:          apiUser,
		APIKey:      apiKey,
		Settings:    user.UserSettings{Timezone: "America/New_York"},
		SystemRoles: []string{"unrestrictedTaskAccess"},
	}
	require.NoError(t, testUser.Insert())
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

	require.NoError(t, setupData(*evergreen.GetEnvironment().DB(), testData))

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

func setupData(db mongo.Database, data map[string]json.RawMessage) error {
	ctx := context.Background()
	catcher := grip.NewBasicCatcher()
	for coll, d := range data {
		var docs []interface{}
		// the docs to insert as part of setup need to be deserialized as extended JSON, whereas the rest of the
		// test spec is normal JSON
		catcher.Add(bson.UnmarshalExtJSON(d, false, &docs))
		_, err := db.Collection(coll).InsertMany(ctx, docs)
		catcher.Add(err)
	}
	return catcher.Resolve()
}
