package graphql

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
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestQueries(t *testing.T, serverURL, pathToTests string) {
	f, err := os.ReadFile(filepath.Join(pathToTests, "integration_spec.json"))
	require.NoError(t, err)
	var spec spec
	err = json.Unmarshal(f, &spec)
	require.NoError(t, err)
	require.NoError(t, spec.setupData(*evergreen.GetEnvironment().DB()))

	for _, testCase := range spec.Tests {
		singleTest := func(t *testing.T) {
			f, err := os.ReadFile(filepath.Join(pathToTests, "testdata", testCase.QueryFile))
			require.NoError(t, err)
			jsonQuery := fmt.Sprintf(`{"operationName":null,"variables":{},"query":"%s"}`, escapeGQLQuery(string(f)))
			body := bytes.NewBuffer([]byte(jsonQuery))
			client := http.Client{}
			r, err := http.NewRequest(http.MethodPost, fmt.Sprintf("%s/graphql/query", serverURL), body)
			require.NoError(t, err)
			r.Header.Add(evergreen.APIKeyHeader, apiKey)
			r.Header.Add(evergreen.APIUserHeader, apiUser)
			r.Header.Add("content-type", "application/json")
			resp, err := client.Do(r)
			require.NoError(t, err)
			b, err := io.ReadAll(resp.Body)
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

		t.Run(testCase.QueryFile, singleTest)
	}
}

type spec struct {
	Setup map[string]json.RawMessage `json:"setupData"`
	Tests []test                     `json:"tests"`
}

func (s *spec) setupData(db mongo.Database) error {
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
