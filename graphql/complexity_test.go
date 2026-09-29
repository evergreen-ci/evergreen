package graphql

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/99designs/gqlgen/graphql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"github.com/vektah/gqlparser/v2/validator"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

const spruceTaskTestsQuery = `
query TaskTests($id: String!, $execution: Int, $pageNum: Int, $limitNum: Int, $statusList: [String!]!, $sort: [TestSortOptions!], $testName: String!) {
  task(taskId: $id, execution: $execution) {
    id
    execution
    tests(opts: {sort: $sort, page: $pageNum, limit: $limitNum, statuses: $statusList, testName: $testName}) {
      filteredTestCount
      testResults {
        id
        baseStatus
        duration
        isManuallyQuarantined
        logs {
          lineNum
          testName
          url
          urlParsley
          urlRaw
        }
        status
        taskId
        testFile
      }
      totalTestCount
    }
  }
}`

const spruceTaskHistoryQuery = `
query TaskHistory($options: TaskHistoryOpts!, $includeGenerator: Boolean!) {
  taskHistory(options: $options) {
    pagination {
      mostRecentTaskOrder
      oldestTaskOrder
    }
    tasks {
      id
      activated
      canRestart
      canSchedule
      canSetPriority
      displayStatus
      execution
      generator @include(if: $includeGenerator) {
        id
        execution
        ingestTime
      }
      ingestTime
      order
      priority
      requester
      revision
      tests(opts: {statuses: ["fail", "silentfail"]}) {
        testResults {
          id
          logs {
            urlParsley
          }
          status
          testFile
        }
      }
      version {
        id
        message
        user {
          id
          displayName
        }
      }
    }
  }
}`

const spruceTaskTestsForJobLogsQuery = `
query TaskTestsForJobLogs($id: String!, $execution: Int) {
  task(taskId: $id, execution: $execution) {
    id
    buildVariant
    buildVariantDisplayName
    displayName
    displayStatus
    execution
    revision
    tests(opts: {}) {
      testResults {
        id
        groupID
        logs {
          urlParsley
        }
        status
        testFile
      }
    }
  }
}`

const spruceTestAnalysisQuery = `
query TestAnalysis($versionId: String!, $options: TaskFilterOptions!, $opts: TestFilterOptions) {
  version(versionId: $versionId) {
    id
    tasks(options: $options) {
      count
      data {
        id
        buildVariant
        buildVariantDisplayName
        displayName
        displayStatus
        execution
        tests(opts: $opts) {
          filteredTestCount
          testResults {
            id
            logs {
              urlParsley
            }
            status
            testFile
          }
        }
      }
    }
  }
}`

// Query shapes from the Sept 18, 2026 traffic that OOMed web pods.
const bulkTaskTestsQuery = `
query {
  task(taskId: "task_id") {
    tests(opts: {testName: "test_a|test_b|test_c", limit: 50}) {
      testResults {
        id
        status
        testFile
        duration
        startTime
        endTime
        baseStatus
        groupID
        exitCode
        execution
        taskId
      }
      totalTestCount
      filteredTestCount
    }
  }
}`

const bulkTaskHistoryTestsQuery = `
query {
  taskHistory(options: {projectIdentifier: "project", taskName: "task", buildVariant: "variant", cursorParams: {cursorId: "task_id", direction: BEFORE, includeCursor: true}, limit: 80}) {
    tasks {
      tests(opts: {testName: "test_a|test_b|test_c", limit: 50}) {
        testResults {
          id
          status
          testFile
          duration
          startTime
          endTime
          baseStatus
          groupID
          exitCode
          execution
          taskId
        }
        totalTestCount
        filteredTestCount
      }
    }
  }
}`

func TestCalculateComplexity(t *testing.T) {
	schema := NewExecutableSchema(New(""))

	parse := func(t *testing.T, query string, vars map[string]any) (*ast.OperationDefinition, map[string]any) {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), query, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		coerced, err := validator.VariableValues(schema.Schema(), doc.Operations[0], vars)
		require.NoError(t, err)
		return doc.Operations[0], coerced
	}
	// testResultsCharge returns the complexity added on top of gqlgen's default.
	testResultsCharge := func(t *testing.T, query string, vars map[string]any) int {
		op, coerced := parse(t, query, vars)
		return calculateComplexity(t.Context(), schema, op, coerced) - complexity.Calculate(t.Context(), schema, op, coerced)
	}

	spruceTaskTestsVars := func(limit int) map[string]any {
		return map[string]any{
			"id":         "task_id",
			"execution":  0,
			"pageNum":    0,
			"limitNum":   limit,
			"statusList": []any{},
			"sort":       []any{map[string]any{"sortBy": "TEST_NAME", "direction": "ASC"}},
			"testName":   "",
		}
	}
	taskHistoryVars := func(limit *int) map[string]any {
		options := map[string]any{
			"projectIdentifier": "project",
			"taskName":          "task",
			"buildVariant":      "variant",
			"cursorParams":      map[string]any{"cursorId": "task_id", "direction": "BEFORE", "includeCursor": true},
		}
		if limit != nil {
			options["limit"] = *limit
		}
		return map[string]any{"options": options, "includeGenerator": false}
	}
	testAnalysisVars := func(limit *int) map[string]any {
		options := map[string]any{"statuses": []any{"failed"}}
		if limit != nil {
			options["limit"] = *limit
		}
		return map[string]any{"versionId": "version_id", "options": options, "opts": map[string]any{"statuses": []any{"fail"}}}
	}
	limit := func(l int) *int { return &l }

	t.Run("TaskTestsIsChargedOnce", func(t *testing.T) {
		assert.Equal(t, testResultsComplexity, testResultsCharge(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)))
		assert.Equal(t, testResultsComplexity, testResultsCharge(t, spruceTaskTestsForJobLogsQuery, map[string]any{"id": "task_id"}))
		assert.Equal(t, testResultsComplexity, testResultsCharge(t, bulkTaskTestsQuery, nil))
	})

	t.Run("TaskTestsChargeDoesNotScaleWithLimit", func(t *testing.T) {
		assert.Equal(t, testResultsCharge(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)), testResultsCharge(t, spruceTaskTestsQuery, spruceTaskTestsVars(100)))
	})

	t.Run("TaskHistoryChargesTestsPerTask", func(t *testing.T) {
		assert.Equal(t, 50*testResultsComplexity, testResultsCharge(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50))))
		assert.Equal(t, 80*testResultsComplexity, testResultsCharge(t, bulkTaskHistoryTestsQuery, nil))
	})

	t.Run("TaskHistoryWithoutLimitUsesUnboundedSize", func(t *testing.T) {
		assert.Equal(t, unboundedTaskListSize*testResultsComplexity, testResultsCharge(t, spruceTaskHistoryQuery, taskHistoryVars(nil)))
		assert.Equal(t, unboundedTaskListSize*testResultsComplexity, testResultsCharge(t, spruceTaskHistoryQuery, taskHistoryVars(limit(0))))
	})

	t.Run("VersionTasksChargesTestsPerTask", func(t *testing.T) {
		assert.Equal(t, 20*testResultsComplexity, testResultsCharge(t, spruceTestAnalysisQuery, testAnalysisVars(limit(20))))
		assert.Equal(t, unboundedTaskListSize*testResultsComplexity, testResultsCharge(t, spruceTestAnalysisQuery, testAnalysisVars(nil)))
	})

	t.Run("TaskListsWithoutTestsAreNotCharged", func(t *testing.T) {
		query := `query { version(versionId: "version_id") { tasks(options: {}) { data { id displayName } } } }`
		assert.Zero(t, testResultsCharge(t, query, nil))
	})

	t.Run("NestedTaskListsMultiply", func(t *testing.T) {
		query := `query { taskHistory(options: {projectIdentifier: "project", taskName: "task", buildVariant: "variant", cursorParams: {cursorId: "task_id", direction: BEFORE, includeCursor: true}, limit: 3}) { tasks { version { tasks(options: {limit: 4}) { data { tests { totalTestCount } } } } } } }`
		assert.Equal(t, 3*4*testResultsComplexity, testResultsCharge(t, query, nil))
	})

	t.Run("FragmentsAreCounted", func(t *testing.T) {
		query := `query { task(taskId: "task_id") { ...TestFields } } fragment TestFields on Task { tests { totalTestCount } }`
		assert.Equal(t, testResultsComplexity, testResultsCharge(t, query, nil))
	})

	t.Run("JSONNumberVariablesAreRead", func(t *testing.T) {
		vars := taskHistoryVars(nil)
		vars["options"].(map[string]any)["limit"] = json.Number("10")
		assert.Equal(t, 10*testResultsComplexity, testResultsCharge(t, spruceTaskHistoryQuery, vars))
	})

	t.Run("HugeLimitDoesNotOverflow", func(t *testing.T) {
		op, coerced := parse(t, spruceTaskHistoryQuery, taskHistoryVars(limit(math.MaxInt)))
		assert.Positive(t, calculateComplexity(t.Context(), schema, op, coerced))
	})

	t.Run("QueriesWithoutTestsUseDefaultComplexity", func(t *testing.T) {
		op, coerced := parse(t, `query { user { displayName } spruceConfig { banner } }`, nil)
		assert.Equal(t, 4, calculateComplexity(t.Context(), schema, op, coerced))
	})
}

func TestComplexityLimit(t *testing.T) {
	schema := NewExecutableSchema(New(""))

	run := func(t *testing.T, limit int, query string) *gqlerror.Error {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), query, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		c := newComplexityLimit(limit)
		require.NoError(t, c.Validate(schema))
		opCtx := &graphql.OperationContext{Doc: doc}
		return c.MutateOperationContext(t.Context(), opCtx)
	}

	t.Run("QueryUnderLimitSucceeds", func(t *testing.T) {
		assert.Nil(t, run(t, 1000, bulkTaskTestsQuery))
	})

	t.Run("QueryOverLimitShouldError", func(t *testing.T) {
		err := run(t, 10, bulkTaskHistoryTestsQuery)
		require.NotNil(t, err)
		assert.Contains(t, err.Message, "exceeds the limit of 10")
		assert.Equal(t, "COMPLEXITY_LIMIT_EXCEEDED", err.Extensions["code"])
	})
}
