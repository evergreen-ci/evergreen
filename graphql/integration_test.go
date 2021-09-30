package graphql_test

// This test takes a specification and runs GraphQL queries, comparing the output of the query to what is expected.
// To add a new test:
// 1. Add any needed setup data to integration_spec.json in the 'setupData' field. This should probably be broken out
//    as individual files in the future.
// 2. Add the query as a .graphql file in the testdata folder
// 3. In integration_spec.json, add a test case to the 'tests' field. List the .graphql file in the 'query_file' field
//    (this will also become the test name) and the expected output in the 'result' field

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type graphQLSuite struct {
	url     string
	apiUser string
	apiKey  string

	suite.Suite
}

func TestGraphQLSuite(t *testing.T) {
	suite.Run(t, &graphQLSuite{})
}

func (s *graphQLSuite) SetupSuite() {
	const apiKey = "testapikey"
	const apiUser = "testuser"

	server, err := service.CreateTestServer(testutil.TestConfig(), nil, true)
	s.Require().NoError(err)
	env := evergreen.GetEnvironment()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.Require().NoError(env.DB().Drop(ctx))
	testUser := user.DBUser{
		Id:          apiUser,
		APIKey:      apiKey,
		Settings:    user.UserSettings{Timezone: "America/New_York"},
		SystemRoles: []string{"unrestrictedTaskAccess"},
	}
	s.Require().NoError(testUser.Insert())
	s.url = server.URL
	s.apiKey = apiKey
	s.apiUser = apiUser
}

func (s *graphQLSuite) TestQueries() {
	f, err := ioutil.ReadFile("integration_spec.json")
	s.Require().NoError(err)
	var spec spec
	err = json.Unmarshal(f, &spec)
	s.Require().NoError(err)
	s.Require().NoError(spec.SetupData(*evergreen.GetEnvironment().DB()))

	for _, testCase := range spec.Tests {
		singleTest := func(t *testing.T) {
			f, err := ioutil.ReadFile(filepath.Join("testdata", testCase.QueryFile))
			require.NoError(t, err)
			jsonQuery := fmt.Sprintf(`{"operationName":null,"variables":{},"query":"%s"}`, escapeGQLQuery(string(f)))
			body := bytes.NewBuffer([]byte(jsonQuery))
			client := http.Client{}
			r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql/query", s.url), body)
			s.Require().NoError(err)
			r.Header.Add(evergreen.APIKeyHeader, s.apiKey)
			r.Header.Add(evergreen.APIUserHeader, s.apiUser)
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

			assert.JSONEq(t, string(testCase.Result), string(b), fmt.Sprintf("expected %s but got %s", string(testCase.Result), string(b)))
		}

		s.T().Run(testCase.QueryFile, singleTest)
	}
}

type spec struct {
	Setup map[string]json.RawMessage `json:"setupData"`
	Tests []test                     `json:"tests"`
}

type test struct {
	QueryFile string          `json:"query_file"`
	Result    json.RawMessage `json:"result"`
}

func (s *spec) SetupData(db mongo.Database) error {
	ctx := context.Background()
	catcher := grip.NewBasicCatcher()
	for coll, data := range s.Setup {
		var docs []interface{}
		// the docs to insert as part of setup need to be deserialized as extended JSON, whereas the rest of the
		// test spec is normal JSON
		catcher.Add(bson.UnmarshalExtJSON(data, false, &docs))
		_, err := db.Collection(coll).InsertMany(ctx, docs)
		catcher.Add(err)
	}
	return catcher.Resolve()
}

// escapeGQLQuery replaces literal newlines with '\n' and literal double quotes with '\"'
func escapeGQLQuery(in string) string {
	return strings.Replace(strings.Replace(in, "\n", "\\n", -1), "\"", "\\\"", -1)
}
