package command

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTestSelectionGet(t *testing.T) {
	t.Run("ParseFailsWithMissingOutputFile", func(t *testing.T) {
		cmd := &testSelectionGet{}
		params := map[string]any{}
		require.Error(t, cmd.ParseParams(params))
	})
	t.Run("SucceedsWithValidParams", func(t *testing.T) {
		cmd := &testSelectionGet{}
		params := map[string]any{
			"output_file": "test.json",
		}
		require.NoError(t, cmd.ParseParams(params))
		assert.Equal(t, "test.json", cmd.OutputFile)
		assert.Empty(t, cmd.Tests)
		assert.Empty(t, cmd.UsageRate)
		assert.Empty(t, cmd.Strategies)

		params["tests"] = []string{"test1", "test2"}
		params["usage_rate"] = "0.5"
		params["strategies"] = "strategy1,strategy2,strategy3"
		require.NoError(t, cmd.ParseParams(params))
		assert.Equal(t, "test.json", cmd.OutputFile)
		assert.Equal(t, []string{"test1", "test2"}, cmd.Tests)
		assert.Equal(t, "0.5", cmd.UsageRate)
		assert.Equal(t, 0.5, cmd.rate)
		assert.Equal(t, "strategy1,strategy2,strategy3", cmd.Strategies)
	})
	t.Run("ParseFailsWithBothTestsListAndTestFile", func(t *testing.T) {
		cmd := &testSelectionGet{}
		params := map[string]any{
			"output_file": "test.json",
			"tests":       []string{"test1", "test2"},
			"tests_file":  "tests.txt",
		}
		assert.Error(t, cmd.ParseParams(params))
	})

	for tName, tCase := range map[string]func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer){
		"BypassRestoresOnlyCandidates": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", `["test2","outside","test2"]`)
			comm.SelectTestsResponse = []string{"test1", "outside"}
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test2", "test1", "test4"}}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Equal(t, []testOutput{{Name: "test2"}, {Name: "test1"}}, output.Tests)
			assert.Equal(t, cmd.Tests, comm.SelectTestsRequest.Tests)
		},
		"BypassFromTestsFileRestoresOnlyCandidates": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", `["test2","outside"]`)
			comm.SelectTestsResponse = []string{"test1", "outside"}
			inputFile := filepath.Join(conf.WorkDir, "input_tests.json")
			require.NoError(t, utility.WriteJSONFile(inputFile, testSelectionInputFile{Tests: []string{"test2", "test1", "test4"}}))
			cmd := &testSelectionGet{OutputFile: "test.json", TestsFile: inputFile}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Equal(t, []testOutput{{Name: "test2"}, {Name: "test1"}}, output.Tests)
			assert.Equal(t, []string{"test2", "test1", "test4"}, comm.SelectTestsRequest.Tests)
		},
		"NonemptyBypassWithoutCandidatesErrors": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", `["test2"]`)
			cmd := &testSelectionGet{OutputFile: "test.json"}
			require.ErrorContains(t, cmd.Execute(t.Context(), comm, logger, conf), "requires nonempty tests or tests_file candidates")
			assert.False(t, comm.SelectTestsCalled)
		},
		"EmptyBypassPreservesKnownTestsMode": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", ` [] `)
			cmd := &testSelectionGet{OutputFile: "test.json"}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Equal(t, []testOutput{{Name: "test1"}, {Name: "test3"}}, output.Tests)
			assert.True(t, comm.SelectTestsCalled)
		},
		"MalformedBypassErrorsBeforeSelection": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			for name, raw := range map[string]string{
				"InvalidJSON": "broken", "Null": "null", "Object": "{}", "String": `"test2"`,
				"NumberElement": "[1]", "NullElement": "[null]", "EmptyElement": `[""]`,
			} {
				t.Run(name, func(t *testing.T) {
					conf.Expansions.Put("test_selection_bypass_quarantine", raw)
					cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test2"}}
					require.ErrorContains(t, cmd.Execute(t.Context(), comm, logger, conf), "test_selection_bypass_quarantine")
					assert.False(t, comm.SelectTestsCalled)
				})
			}
		},
		"MainlineIgnoresMalformedBypass": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Task.Requester = evergreen.RepotrackerVersionRequester
			conf.ProjectRef.TestSelection.DefaultEnabled = utility.TruePtr()
			conf.ProjectRef.TestSelection.MainlineDefaultEnabled = utility.TruePtr()
			conf.Expansions.Put("test_selection_bypass_quarantine", "broken")
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1"}}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Equal(t, []testOutput{{Name: "test1"}, {Name: "test3"}}, output.Tests)
			assert.True(t, comm.SelectTestsCalled)
		},
		"DisabledSelectionAndUsageNoopIgnoreMalformedBypass": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", "broken")
			for _, name := range []string{"Disabled", "ZeroUsage", "SampledNoop"} {
				t.Run(name, func(t *testing.T) {
					conf.ProjectRef.TestSelection.Allowed = utility.ToBoolPtr(name != "Disabled")
					cmd := &testSelectionGet{OutputFile: "test.json"}
					if name == "ZeroUsage" {
						cmd.UsageRate = "0"
					} else if name == "SampledNoop" {
						cmd.UsageRate = "1"
					}
					require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
					var output testSelectionOutputFile
					require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
					assert.Empty(t, output.Tests)
					assert.False(t, comm.SelectTestsCalled)
				})
			}
		},
		"APIErrorStillErrorsWithBypass": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			conf.Expansions.Put("test_selection_bypass_quarantine", `["test2"]`)
			comm.SelectTestsError = errors.New("selection error")
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test2"}}
			require.ErrorContains(t, cmd.Execute(t.Context(), comm, logger, conf), "calling test selection API: selection error")
			assert.True(t, comm.SelectTestsCalled)
		},
		"BypassPreservesExactNamesAndCandidateDuplicates": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			names := []string{"a,b.js", "a b.js", `a"b.js`, `a\b.js`, "a=b.js", "$(name).js", " "}
			raw, err := json.Marshal(names)
			require.NoError(t, err)
			conf.Expansions.Put("test_selection_bypass_quarantine", string(raw))
			comm.SelectTestsResponse = nil
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: append(names, "a,b.js", "not-selected")}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))
			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Equal(t, []testOutput{
				{Name: "a,b.js"}, {Name: "a b.js"}, {Name: `a"b.js`}, {Name: `a\b.js`},
				{Name: "a=b.js"}, {Name: "$(name).js"}, {Name: " "}, {Name: "a,b.js"},
			}, output.Tests)
		},
		"SkipsWhenTestSelectionNotAllowed": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json"}

			// Test selection not allowed in project settings
			conf.ProjectRef.TestSelection.Allowed = utility.FalsePtr()
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Empty(t, output.Tests)

			// Test selection allowed in project settings but not enabled for task
			conf.ProjectRef.TestSelection.Allowed = utility.TruePtr()
			conf.Task.TestSelectionEnabled = false
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			output = testSelectionOutputFile{}
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Empty(t, output.Tests)
		},
		"SkipsForMainlineRequesterByDefault": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

			conf.Task.Requester = evergreen.RepotrackerVersionRequester
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.False(t, comm.SelectTestsCalled)

			var output testSelectionOutputFile
			require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
			assert.Empty(t, output.Tests)
		},
		"CallsTestSelectionAPIForMainlineRequesterWhenEnabled": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

			conf.Task.Requester = evergreen.RepotrackerVersionRequester
			conf.ProjectRef.TestSelection.DefaultEnabled = utility.TruePtr()
			conf.ProjectRef.TestSelection.MainlineDefaultEnabled = utility.TruePtr()
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.True(t, comm.SelectTestsCalled)
			assert.Equal(t, evergreen.RepotrackerVersionRequester, comm.SelectTestsRequest.Requester)
		},
		"SkipsForOtherNonPatchRequester": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

			conf.Task.Requester = evergreen.AdHocRequester
			conf.ProjectRef.TestSelection.MainlineDefaultEnabled = utility.TruePtr()
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.False(t, comm.SelectTestsCalled)
		},
		"CallsTestSelectionAPIWhenEnabled": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			// Should return the expected tests from the mock API.
			data, err := os.ReadFile(cmd.OutputFile)
			require.NoError(t, err)
			var output testSelectionOutputFile
			require.NoError(t, json.Unmarshal(data, &output))
			require.Len(t, output.Tests, 2)
			assert.Equal(t, "test1", output.Tests[0].Name)
			assert.Equal(t, "test3", output.Tests[1].Name)

			// Verify the API was called with correct parameters from TaskConfig
			assert.True(t, comm.SelectTestsCalled)
			assert.Equal(t, conf.Task.Project, comm.SelectTestsRequest.Project)
			assert.Equal(t, conf.Task.Requester, comm.SelectTestsRequest.Requester)
			assert.Equal(t, conf.Task.BuildVariant, comm.SelectTestsRequest.BuildVariant)
			assert.Equal(t, conf.Task.Id, comm.SelectTestsRequest.TaskID)
			assert.Equal(t, conf.Task.DisplayName, comm.SelectTestsRequest.TaskName)
			assert.Equal(t, []string{"test1", "test3"}, comm.SelectTestsRequest.Tests)
		},
		"PassesTestsToAPI": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			// Should return the expected tests from the mock API.
			data, err := os.ReadFile(cmd.OutputFile)
			require.NoError(t, err)
			var output testSelectionOutputFile
			require.NoError(t, json.Unmarshal(data, &output))
			require.Len(t, output.Tests, 2)
			assert.Equal(t, "test1", output.Tests[0].Name)
			assert.Equal(t, "test3", output.Tests[1].Name)

			// Verify the API was called with correct parameters from TaskConfig
			assert.True(t, comm.SelectTestsCalled)
			assert.Equal(t, conf.Task.Project, comm.SelectTestsRequest.Project)
			assert.Equal(t, conf.Task.Requester, comm.SelectTestsRequest.Requester)
			assert.Equal(t, conf.Task.BuildVariant, comm.SelectTestsRequest.BuildVariant)
			assert.Equal(t, conf.Task.Id, comm.SelectTestsRequest.TaskID)
			assert.Equal(t, conf.Task.DisplayName, comm.SelectTestsRequest.TaskName)
			assert.Equal(t, []string{"test1", "test3"}, comm.SelectTestsRequest.Tests)
		},
		"PassesTestsFromFileToAPI": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			tests := []string{"test1", "test2", "test3", "test4"}
			inputTests := testSelectionInputFile{
				Tests: tests,
			}
			inputFile := filepath.Join(conf.WorkDir, "input_tests.json")
			require.NoError(t, utility.WriteJSONFile(inputFile, inputTests))

			cmd := &testSelectionGet{OutputFile: "test.json", TestsFile: inputFile}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.True(t, comm.SelectTestsCalled)
			assert.Equal(t, tests, comm.SelectTestsRequest.Tests)
		},
		"HandlesAPIErrors": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json"}

			// Mock API error
			comm.SelectTestsError = errors.New("test error")

			err := cmd.Execute(t.Context(), comm, logger, conf)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "test error")
			assert.Contains(t, err.Error(), "calling test selection API")
		},
		"AlwaysRunsTestSelectionWhenUsageRateIsSetToAnUndefinedExpansion": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", UsageRate: "${undefined_expansion}"}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.True(t, comm.SelectTestsCalled)
		},
		"NeverRunsTestSelectionWhenUsageRateIsExplicitlySetToZero": func(t *testing.T, conf *internal.TaskConfig, comm *client.Mock, logger client.LoggerProducer) {
			cmd := &testSelectionGet{OutputFile: "test.json", UsageRate: "0"}
			require.NoError(t, cmd.Execute(t.Context(), comm, logger, conf))

			assert.False(t, comm.SelectTestsCalled)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			comm := client.NewMock("http://localhost.com")
			comm.SelectTestsResponse = []string{"test1", "test3"}
			conf := &internal.TaskConfig{
				Task: task.Task{
					Id:           "task_id",
					Secret:       "task_secret",
					Project:      "project_id",
					Version:      "version",
					BuildVariant: "build_variant",
					DisplayName:  "display_name",
				},
				ProjectRef: model.ProjectRef{Id: "project_id"},
			}
			tmpDir := t.TempDir()
			conf.WorkDir = tmpDir
			conf.Expansions = *util.NewExpansions(map[string]string{})
			f, err := os.Create(filepath.Join(tmpDir, "test.json"))
			require.NoError(t, err)
			require.NoError(t, f.Close())

			logger, err := comm.GetLoggerProducer(t.Context(), &conf.Task, nil)
			require.NoError(t, err)

			conf.Task.TestSelectionEnabled = true
			conf.Task.Requester = evergreen.PatchVersionRequester
			conf.ProjectRef.TestSelection.Allowed = utility.TruePtr()

			tCase(t, conf, comm, logger)
		})
	}
}
