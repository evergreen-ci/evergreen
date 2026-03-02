package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParserRegex(t *testing.T) {
	t.Run("TestStartVanillaLogs", func(t *testing.T) {
		name, err := startInfoFromLogLine("=== RUN   TestParserFunctionality", startRegex)
		require.NoError(t, err)
		assert.Equal(t, "TestParserFunctionality", name)
	})
	t.Run("TestStartGocheckLogs", func(t *testing.T) {
		name, err := startInfoFromLogLine("START: test_file.go:81: TestName.TearDownSuite", gocheckStartRegex)
		require.NoError(t, err)
		assert.Equal(t, "TestName.TearDownSuite", name)
	})
	t.Run("TestEndVanillaLogs", func(t *testing.T) {
		name, status, dur, err := endInfoFromLogLine("--- FAIL: TestParserRegex (0.05s)", endRegex)
		require.NoError(t, err)
		assert.Equal(t, "TestParserRegex", name)
		assert.Equal(t, FAIL, status)
		assert.Equal(t, time.Duration(50)*time.Millisecond, dur)

		name, status, dur, err = endInfoFromLogLine("--- PASS: TestParserRegex (0.00s)", endRegex)
		require.NoError(t, err)
		assert.Equal(t, "TestParserRegex", name)
		assert.Equal(t, PASS, status)
		assert.Equal(t, time.Duration(0), dur)

		name, status, dur, err = endInfoFromLogLine("--- PASS: TestParserRegex (2m4.0s)", endRegex)
		require.NoError(t, err)
		assert.Equal(t, "TestParserRegex", name)
		assert.Equal(t, PASS, status)
		expDur, err := time.ParseDuration("2m4s")
		require.NoError(t, err)
		assert.Equal(t, expDur, dur)
	})
	t.Run("TestEndGocheckLogs", func(t *testing.T) {
		name, status, dur, err := endInfoFromLogLine(
			"FAIL: adjust_test.go:40: AdjustSuite.TestAdjust", gocheckEndRegex)
		require.NoError(t, err)
		assert.Equal(t, "AdjustSuite.TestAdjust", name)
		assert.Equal(t, FAIL, status)
		assert.Equal(t, time.Duration(0), dur)

		name, status, dur, err = endInfoFromLogLine(
			"PASS: update_test.go:81: UpdateSuite.TearDownSuite	0.900s", gocheckEndRegex)
		require.NoError(t, err)
		assert.Equal(t, "UpdateSuite.TearDownSuite", name)
		assert.Equal(t, PASS, status)
		assert.Equal(t, time.Duration(900)*time.Millisecond, dur)
	})
	t.Run("GoTestBuildLines", func(t *testing.T) {
		path, err := pathNameFromLogLine(
			"FAIL   github.go/evergreen-ci/evergreen/model/patch.go     [build failed] ")
		require.NoError(t, err)
		assert.Equal(t, "github.go/evergreen-ci/evergreen/model/patch.go", path)

		path, err = pathNameFromLogLine(
			"FAIL github.go/evergreen-ci/evergreen/model/patch.go [build failed]")
		require.NoError(t, err)
		assert.Equal(t, "github.go/evergreen-ci/evergreen/model/patch.go", path)

		_, err = pathNameFromLogLine(
			"FAIL   github.go/evergreen-ci/evergreen/model/patch.go 2.47s")
		assert.Error(t, err)

		_, err = pathNameFromLogLine(
			"ok     github.go/evergreen-ci/evergreen/model/patch.go 2.47s")
		assert.Error(t, err)
	})
}

func TestParserFunctionality(t *testing.T) {
	cwd := testutil.GetDirectoryOfFile()

	t.Run("SimpleLogFile", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "1_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		assert.Len(t, parser.Logs(), 18)

		results := parser.Results()
		require.Len(t, results, 2)

		assert.Equal(t, "TestFailures", results[0].Name)
		assert.Equal(t, FAIL, results[0].Status)
		rTime, err := time.ParseDuration("5.02s")
		require.NoError(t, err)
		assert.Equal(t, rTime, results[0].RunTime)
		assert.Equal(t, 1, results[0].StartLine)
		assert.Equal(t, 14, results[0].EndLine)

		assert.Equal(t, "TestFailures2", results[1].Name)
		assert.Equal(t, FAIL, results[1].Status)
		rTime, err = time.ParseDuration("2.00s")
		require.NoError(t, err)
		assert.Equal(t, rTime, results[1].RunTime)
		assert.Equal(t, 15, results[1].StartLine)
		assert.Equal(t, 15, results[1].EndLine)
	})
	t.Run("GocheckLogFile", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "2_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		assert.Len(t, parser.Logs(), 15)

		results := parser.Results()
		require.Len(t, results, 3)

		assert.Equal(t, "MyTestName.SetUpTest", results[1].Name)
		assert.Equal(t, PASS, results[1].Status)
		rTime, err := time.ParseDuration("0.576s")
		require.NoError(t, err)
		assert.Equal(t, rTime, results[1].RunTime)
		assert.Equal(t, 2, results[1].StartLine)
		assert.Equal(t, 4, results[1].EndLine)
	})
	t.Run("UnterminatedTestsAreFailures", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "3_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 1)
		assert.Equal(t, "TestFailures", results[0].Name)
		assert.Equal(t, FAIL, results[0].Status)
	})
	t.Run("TestifySuitesWithLeadingSpaces", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "4_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 19)
		assert.Equal(t, "TestClientSuite/TestURLGeneratiorWithoutDefaultPortInResult", results[18].Name)
		assert.Equal(t, PASS, results[18].Status)
	})
	t.Run("MultipleExecutionsOfSameTest", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "5_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 3)
		assert.Equal(t, "Test1", results[0].Name)
		assert.Equal(t, "TestSameName", results[1].Name)
		assert.Equal(t, "TestSameName", results[2].Name)
		assert.Equal(t, PASS, results[1].Status)
		assert.Equal(t, FAIL, results[2].Status)
	})
	t.Run("NegativeDuration", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "6_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 1)
		assert.Equal(t, PASS, results[0].Status)
	})
	t.Run("DeeplyNestedSubtests", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "7_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 39)
		for idx, r := range results {
			if idx == 0 {
				continue
			}
			outcome := strings.Contains(r.Name, "Basic") || strings.Contains(r.Name, "Complex")
			assert.True(t, outcome, "result %q should contain either 'Basic' or 'Complex'", r.Name)
		}
	})
	t.Run("FailedBuild", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "8_simple.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		require.NoError(t, parser.Parse(bytes.NewBuffer(logdata)))

		results := parser.Results()
		require.Len(t, results, 4) // 3 passing tests + 1 build failure
		buildFailed := results[3]
		assert.Equal(t, "[build failed] github.com/evergreen-ci/evergreen/model/host", buildFailed.Name)
		assert.Equal(t, FAIL, buildFailed.Status)
	})
	t.Run("LargeLogLine", func(t *testing.T) {
		logdata, err := os.ReadFile(filepath.Join(cwd, "testdata", "gotest", "large_line.log"))
		require.NoError(t, err)

		parser := &goTestParser{}
		assert.Error(t, parser.Parse(bytes.NewBuffer(logdata)))
	})
}
