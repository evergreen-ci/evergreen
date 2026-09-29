package graphql

import (
	"encoding/json"
	"testing"

	"github.com/99designs/gqlgen/complexity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/vektah/gqlparser/v2"
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

func TestComplexity(t *testing.T) {
	schema := newSchema("")

	calculate := func(t *testing.T, query string, vars map[string]any) int {
		doc, gqlErrs := gqlparser.LoadQueryWithRules(schema.Schema(), query, rules.NewDefaultRules())
		require.Empty(t, gqlErrs)
		require.Len(t, doc.Operations, 1)
		coerced, err := validator.VariableValues(schema.Schema(), doc.Operations[0], vars)
		require.NoError(t, err)
		return complexity.Calculate(t.Context(), schema, doc.Operations[0], coerced)
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

	t.Run("TaskTestsChargesFlatComplexity", func(t *testing.T) {
		assert.Equal(t, 3+testResultsComplexity+16, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)))
	})

	t.Run("TaskTestsDoesNotScaleWithLimit", func(t *testing.T) {
		assert.Equal(t, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)), calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(100)))
		assert.Equal(t, calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(10)), calculate(t, spruceTaskTestsQuery, spruceTaskTestsVars(0)))
	})

	t.Run("TaskTestsWithoutOptsChargesFlatComplexity", func(t *testing.T) {
		assert.Equal(t, 8+testResultsComplexity+7, calculate(t, spruceTaskTestsForJobLogsQuery, map[string]any{"id": "task_id"}))
	})

	t.Run("TaskHistoryScalesWithLimit", func(t *testing.T) {
		smaller := calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(10)))
		larger := calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50)))
		assert.Equal(t, 5*(smaller-1), larger-1)
	})

	t.Run("TaskHistoryWithoutLimitUsesUnboundedSize", func(t *testing.T) {
		assert.Equal(t, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(unboundedTaskListSize))), calculate(t, spruceTaskHistoryQuery, taskHistoryVars(nil)))
		assert.Equal(t, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(unboundedTaskListSize))), calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(0))))
	})

	t.Run("SpruceTaskHistoryChargesTestsPerTask", func(t *testing.T) {
		perTask := 12 + 4 + testResultsComplexity + 6 + 6
		assert.Equal(t, 1+(3+1+perTask)*50, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(50))))
	})

	t.Run("VersionTasksWithoutLimitUsesUnboundedSize", func(t *testing.T) {
		perTask := 6 + testResultsComplexity + 7
		assert.Equal(t, 1+1+1+(1+1+perTask)*unboundedTaskListSize, calculate(t, spruceTestAnalysisQuery, testAnalysisVars(nil)))
	})

	t.Run("VersionTasksScalesWithLimit", func(t *testing.T) {
		smaller := calculate(t, spruceTestAnalysisQuery, testAnalysisVars(limit(10)))
		larger := calculate(t, spruceTestAnalysisQuery, testAnalysisVars(limit(20)))
		assert.Equal(t, 2*(smaller-3), larger-3)
	})

	t.Run("BulkTaskTestsQuery", func(t *testing.T) {
		assert.Equal(t, 1+testResultsComplexity+14, calculate(t, bulkTaskTestsQuery, nil))
	})

	t.Run("BulkTaskHistoryTestsQuery", func(t *testing.T) {
		assert.Equal(t, 1+(1+testResultsComplexity+14)*80, calculate(t, bulkTaskHistoryTestsQuery, nil))
	})

	t.Run("JSONNumberVariablesAreRead", func(t *testing.T) {
		vars := taskHistoryVars(nil)
		vars["options"].(map[string]any)["limit"] = json.Number("10")
		assert.Equal(t, calculate(t, spruceTaskHistoryQuery, taskHistoryVars(limit(10))), calculate(t, spruceTaskHistoryQuery, vars))
	})

	t.Run("UnlistedFieldUsesDefaultComplexity", func(t *testing.T) {
		assert.Equal(t, 4, calculate(t, `query { user { displayName } spruceConfig { banner } }`, nil))
	})
}
