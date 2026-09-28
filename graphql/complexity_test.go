package graphql

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
	"github.com/vektah/gqlparser/v2/validator"
	"github.com/vektah/gqlparser/v2/validator/rules"
)

// The Spruce queries below are copied from the Spruce UI's task-tests.ts,
// task-history.ts, and task-tests-for-job-logs.ts queries.
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

// The two queries below are the shapes of the high-volume traffic that caused
// web pods to run out of memory on Sept 18, 2026.
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

func TestComplexity(t *testing.T) {
	schema := NewSchema("")

	calculate := func(t *testing.T, query string, vars map[string]any) int {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), query, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		return complexity.Calculate(t.Context(), schema, doc.Operations[0], vars)
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
	limit := func(l int) *int { return &l }

	t.Run("TaskTestsScalesWithLimit", func(t *testing.T) {
		// task, id, and execution (3) + tests (1 + 16 child fields * limit).
		assert.Equal(t, 3+1+16*10, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)))
		assert.Equal(t, 3+1+16*100, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(100)))
	})

	t.Run("TaskTestsWithoutLimitUsesUnboundedMultiplier", func(t *testing.T) {
		vars := map[string]any{"id": "task_id", "execution": 0}
		// task and its 7 scalar fields (8) + tests (1 + 7 child fields * 100).
		assert.Equal(t, 8+1+7*100, calculate(t, spruceTaskTestsForJobLogsQuery, vars))
	})

	t.Run("TaskTestsWithZeroLimitUsesUnboundedMultiplier", func(t *testing.T) {
		assert.Equal(t, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(100)), calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(0)))
	})

	t.Run("TaskHistoryScalesWithLimit", func(t *testing.T) {
		smaller := calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(10)))
		larger := calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50)))
		assert.Equal(t, 5*(smaller-1), larger-1)
	})

	t.Run("TaskHistoryWithoutLimitUsesSchemaDefault", func(t *testing.T) {
		assert.Equal(t, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50))), calculate(t, spruceTaskHistoryQuery, taskHistoryVars(nil)))
	})

	t.Run("SpruceTaskHistoryMultipliesNestedTestsByTaskLimit", func(t *testing.T) {
		// Each task's tests field requests no limit, so it is scored with the
		// unbounded multiplier (1 + 6 child fields * 100), and that is multiplied
		// again by the 50 tasks along with the other 22 fields per task and the 3
		// pagination fields. The complexity calculation does not evaluate
		// @include, so the generator fields are always counted.
		testsComplexity := 1 + 6*100
		assert.Equal(t, 1+(3+1+22+testsComplexity)*50, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50))))
	})

	t.Run("BulkTaskTestsQuery", func(t *testing.T) {
		// task (1) + tests (1 + 14 child fields * 50).
		assert.Equal(t, 1+1+14*50, calculate(t, bulkTaskTestsQuery, nil))
	})

	t.Run("BulkTaskHistoryTestsQuery", func(t *testing.T) {
		// taskHistory (1 + tasks' complexity * 80), where each task's tests
		// field is scored as in BulkTaskTestsQuery.
		assert.Equal(t, 1+(1+(1+14*50))*80, calculate(t, bulkTaskHistoryTestsQuery, nil))
	})

	t.Run("HugeLimitDoesNotOverflow", func(t *testing.T) {
		assert.Equal(t, math.MaxInt, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(math.MaxInt)))
	})

	t.Run("ZeroLimitUsesResolverFallback", func(t *testing.T) {
		query := `query { mainlineCommits(options: {projectIdentifier: "project", limit: 0}) { versions { version { id } } } }`
		// mainlineCommits (1 + (versions (1) + version (1) + id (1)) * 7).
		assert.Equal(t, 1+3*7, calculate(t, query, nil))
	})

	t.Run("JSONNumberVariablesAreRead", func(t *testing.T) {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), spruceTaskTestsQuery, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		vars := spruceTaskTestsVars(0)
		vars["limitNum"] = json.Number("10")
		coerced, err := validator.VariableValues(schema.Schema(), doc.Operations[0], vars)
		require.NoError(t, err)
		assert.Equal(t, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)), complexity.Calculate(t.Context(), schema, doc.Operations[0], coerced))
	})

	t.Run("UnlistedFieldUsesDefaultComplexity", func(t *testing.T) {
		// user (1) + displayName (1) + spruceConfig (1) + banner (1).
		assert.Equal(t, 4, calculate(t, `query { user { displayName } spruceConfig { banner } }`, nil))
	})
}

func TestListLimitDefaults(t *testing.T) {
	schema := NewExecutableSchema(New("")).Schema()

	t.Run("AllListLimitFieldsResolveAgainstSchema", func(t *testing.T) {
		defaults, err := listLimitDefaults(schema, listLimitFields)
		require.NoError(t, err)
		require.NotNil(t, defaults["Query.taskHistory"])
		assert.Equal(t, 50, *defaults["Query.taskHistory"])
		assert.Nil(t, defaults["Task.tests"])
	})

	t.Run("MissingFieldShouldError", func(t *testing.T) {
		_, err := listLimitDefaults(schema, map[string]listLimit{"Task.nonexistent": {arg: "limit"}})
		assert.Error(t, err)
	})

	t.Run("MissingArgumentPathShouldError", func(t *testing.T) {
		_, err := listLimitDefaults(schema, map[string]listLimit{"Task.tests": {arg: "opts.nonexistent"}})
		assert.Error(t, err)
	})

	t.Run("NonIntArgumentShouldError", func(t *testing.T) {
		_, err := listLimitDefaults(schema, map[string]listLimit{"Task.tests": {arg: "opts.testName"}})
		assert.Error(t, err)
	})
}
