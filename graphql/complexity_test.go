package graphql

import (
	"strings"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

const taskTestsQuery = `
query TaskTests($limitNum: Int) {
  task(taskId: "t", execution: 0) {
    id
    tests(opts: { limit: $limitNum, statuses: [], testName: "" }) {
      filteredTestCount
      testResults {
        id
        status
        testFile
        logs {
          urlParsley
        }
      }
      totalTestCount
    }
  }
}`

const taskTestsNoOptsQuery = `
query {
  task(taskId: "t", execution: 0) {
    tests {
      testResults {
        id
      }
    }
  }
}`

func TestComplexity(t *testing.T) {
	schema := NewExecutableSchema(New(""))

	calculate := func(t *testing.T, query string, vars map[string]any) int {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), query, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		return complexity.Calculate(t.Context(), schema, doc.Operations[0], vars)
	}

	t.Run("TaskTestsIncludesFetchCost", func(t *testing.T) {
		score := calculate(t, taskTestsQuery, map[string]any{"limitNum": 1})
		assert.Greater(t, score, 2*testResultsFetchComplexity)
	})
	t.Run("TaskTestsScalesWithLimit", func(t *testing.T) {
		small := calculate(t, taskTestsQuery, map[string]any{"limitNum": 10})
		large := calculate(t, taskTestsQuery, map[string]any{"limitNum": 100})
		assert.Greater(t, large, small)
	})
	t.Run("TaskTestsWithoutLimitScoresAsUnbounded", func(t *testing.T) {
		unbounded := calculate(t, taskTestsQuery, map[string]any{"limitNum": nil})
		bounded := calculate(t, taskTestsQuery, map[string]any{"limitNum": defaultTestResultsCount})
		assert.Equal(t, bounded, unbounded)
	})
	t.Run("TaskTestsWithoutOptsExcludesBaseTaskFetch", func(t *testing.T) {
		withoutOpts := calculate(t, taskTestsNoOptsQuery, nil)
		withOpts := calculate(t, strings.Replace(taskTestsNoOptsQuery, "tests {", "tests(opts: {}) {", 1), nil)
		assert.Equal(t, testResultsFetchComplexity, withOpts-withoutOpts)
	})
}
