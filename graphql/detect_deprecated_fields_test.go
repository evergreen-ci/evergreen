package graphql

import (
	"context"
	"testing"

	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
)

const deprecationTestSchema = `
type Query {
	task(taskId: String, oldArg: String @deprecated(reason: "Use taskId."), filter: Filter, filters: [Filter!]): Task
	oldQuery: String @deprecated(reason: "Use task.")
}

type Mutation {
	doThing(input: Filter): Task @deprecated(reason: "Use doOtherThing.")
}

type Task {
	id: String!
	displayName: String
	legacyName: String @deprecated(reason: "Use displayName.")
}

input Filter {
	name: String
	oldName: String @deprecated(reason: "Use name.")
	nested: Nested
}

input Nested {
	inner: String
	oldInner: String @deprecated(reason: "Use inner.")
}
`

func TestDetectDeprecatedFieldsInterceptResponse(t *testing.T) {
	schema := gqlparser.MustLoadSchema(&ast.Source{Name: "schema.graphql", Input: deprecationTestSchema})

	for testName, testCase := range map[string]struct {
		query     string
		variables map[string]any
		expected  []deprecatedField
	}{
		"QueryWithoutDeprecatedElementsHasNoDeprecationExtension": {
			query: `{ task(taskId: "a", filter: {name: "x"}) { id displayName } }`,
		},
		"UnsuppliedVariableHasNoDeprecationExtension": {
			query: `query($f: Filter) { task(filter: $f) { id } }`,
		},
		"DeprecatedQuery": {
			query:    `{ oldQuery }`,
			expected: []deprecatedField{{Field: "Query.oldQuery", Reason: "Use task."}},
		},
		"DeprecatedMutation": {
			query:    `mutation { doThing { id } }`,
			expected: []deprecatedField{{Field: "Mutation.doThing", Reason: "Use doOtherThing."}},
		},
		"DeprecatedSubfield": {
			query:    `{ task { legacyName } }`,
			expected: []deprecatedField{{Field: "Task.legacyName", Reason: "Use displayName."}},
		},
		"DeprecatedFieldInNamedFragment": {
			query:    `{ task { ...F } } fragment F on Task { legacyName }`,
			expected: []deprecatedField{{Field: "Task.legacyName", Reason: "Use displayName."}},
		},
		"DeprecatedFieldInInlineFragment": {
			query:    `{ task { ... on Task { legacyName } } }`,
			expected: []deprecatedField{{Field: "Task.legacyName", Reason: "Use displayName."}},
		},
		"DeprecatedArgument": {
			query:    `{ task(oldArg: "a") { id } }`,
			expected: []deprecatedField{{Field: "Query.task.oldArg", Reason: "Use taskId."}},
		},
		"DeprecatedInputField": {
			query:    `{ task(filter: {oldName: "x"}) { id } }`,
			expected: []deprecatedField{{Field: "Filter.oldName", Reason: "Use name."}},
		},
		"DeprecatedInputFieldInVariable": {
			query:     `query($f: Filter) { task(filter: $f) { id } }`,
			variables: map[string]any{"f": map[string]any{"oldName": "x"}},
			expected:  []deprecatedField{{Field: "Filter.oldName", Reason: "Use name."}},
		},
		"DeprecatedInputFieldInList": {
			query:    `{ task(filters: [{name: "a"}, {oldName: "b"}]) { id } }`,
			expected: []deprecatedField{{Field: "Filter.oldName", Reason: "Use name."}},
		},
		"DeprecatedInputFieldInListVariable": {
			query: `query($fs: [Filter!]) { task(filters: $fs) { id } }`,
			variables: map[string]any{"fs": []any{
				map[string]any{"name": "a"},
				map[string]any{"oldName": "b"},
			}},
			expected: []deprecatedField{{Field: "Filter.oldName", Reason: "Use name."}},
		},
		"DeprecatedInputFieldInNestedInputObject": {
			query:     `query($f: Filter) { task(filter: $f) { id } }`,
			variables: map[string]any{"f": map[string]any{"nested": map[string]any{"oldInner": "x"}}},
			expected:  []deprecatedField{{Field: "Nested.oldInner", Reason: "Use inner."}},
		},
		"DeprecatedMutationAndDeprecatedInputField": {
			query:     `mutation($f: Filter) { doThing(input: $f) { id } }`,
			variables: map[string]any{"f": map[string]any{"oldName": "x"}},
			expected: []deprecatedField{
				{Field: "Mutation.doThing", Reason: "Use doOtherThing."},
				{Field: "Filter.oldName", Reason: "Use name."},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			doc, errs := gqlparser.LoadQueryWithRules(schema, testCase.query, nil)
			require.Empty(t, errs)
			require.Len(t, doc.Operations, 1)

			ctx := graphql.WithOperationContext(t.Context(), &graphql.OperationContext{Operation: doc.Operations[0], Variables: testCase.variables})
			ctx = graphql.WithResponseContext(ctx, graphql.DefaultErrorPresenter, graphql.DefaultRecover)

			ext := &DetectDeprecatedFields{schema: schema}
			resp := ext.InterceptResponse(ctx, func(ctx context.Context) *graphql.Response {
				return &graphql.Response{Extensions: graphql.GetExtensions(ctx)}
			})

			require.NotNil(t, resp)
			if testCase.expected == nil {
				assert.NotContains(t, resp.Extensions, "deprecatedFields")
				return
			}
			require.Contains(t, resp.Extensions, "deprecatedFields")
			assert.ElementsMatch(t, testCase.expected, resp.Extensions["deprecatedFields"])
		})
	}
}
