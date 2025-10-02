package command

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"fmt"
	"io"
	"math/rand"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const (
	testSelectionGetAttribute = "evergreen.command.test_selection.get"
)

var (
	testSelectionEnabledAttribute          = fmt.Sprintf("%s.enabled", testSelectionGetAttribute)
	testSelectionCalledAttribute           = fmt.Sprintf("%s.called", testSelectionGetAttribute)
	testSelectionInputTestsAttribute       = fmt.Sprintf("%s.input_tests", testSelectionGetAttribute)
	testSelectionStrategiesAttribute       = fmt.Sprintf("%s.strategies", testSelectionGetAttribute)
	testSelectionUsageRateAttribute        = fmt.Sprintf("%s.usage_rate", testSelectionGetAttribute)
	testSelectionNumTestsReturnedAttribute = fmt.Sprintf("%s.num_tests_returned", testSelectionGetAttribute)
	testSelectionDurationMsAttribute       = fmt.Sprintf("%s.duration_ms", testSelectionGetAttribute)
)

type testSelectionInputFile struct {
	Tests []string `json:"tests"`
}

type testOutput struct {
	Name string `json:"name"`
}

type testSelectionOutputFile struct {
	Tests []testOutput `json:"tests"`
}

type testSelectionGet struct {
	// OutputFile is the path to where the JSON file should be written.
	// Required.
	OutputFile string `mapstructure:"output_file" plugin:"expand"`

	// Tests is a list of test names to pass into the TSS API.
	// Optional.
	Tests []string `mapstructure:"tests" plugin:"expand"`

	// TestsFile is an path to a file containing a JSON array of test names.
	// Optional.
	TestsFile string `mapstructure:"tests_file" plugin:"expand"`

	// UsageRate is an optional string that specifies a proportion
	// between 0 and 1 of how often to actually use test selection.
	// For example, if usage_rate is 0.4, it'll actually request a list of
	// recommended tests 40% of the time; otherwise, it'll be a no-op.
	// The default is 1 (i.e. always request test selection).
	UsageRate string `mapstructure:"usage_rate" plugin:"expand"`

	// rate is the parsed float value of UsageRate.
	rate float64

	// Strategies is an optional comma-separated string that specifies
	// a comma-separated list of strategy names to use.
	Strategies string `mapstructure:"strategies" plugin:"expand"`

	base
}

func testSelectionGetFactory() Command   { return &testSelectionGet{} }
func (c *testSelectionGet) Name() string { return "test_selection.get" }

func (c *testSelectionGet) ParseParams(params map[string]any) error {
	if err := mapstructure.Decode(params, c); err != nil {
		return errors.Wrap(err, "decoding mapstructure params")
	}

	return c.validate()
}

func (c *testSelectionGet) validate() error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(c.OutputFile == "", "must specify output file")
	if c.UsageRate != "" {
		rate, err := strconv.ParseFloat(c.UsageRate, 64)
		catcher.Add(err)
		catcher.NewWhen(rate < 0 || rate > 1, "usage rate must be between 0 and 1")
		c.rate = rate
	}
	catcher.NewWhen(len(c.Tests) > 0 && c.TestsFile != "", "cannot specify both tests and tests_file")
	return catcher.Resolve()
}

func (c *testSelectionGet) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	calledAPI := false
	defer func() {
		trace.SpanFromContext(ctx).SetAttributes(attribute.Bool(testSelectionCalledAttribute, calledAPI))
	}()

	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// Re-validate the command here, in case an expansion is not defined.
	if err := c.validate(); err != nil {
		return errors.Wrap(err, "validating command")
	}

	// Resolve the output file path early so it's available for writing empty results.
	if !filepath.IsAbs(c.OutputFile) {
		c.OutputFile = GetWorkingDirectory(conf, c.OutputFile)
	}

	if c.TestsFile != "" && !filepath.IsAbs(c.TestsFile) {
		c.TestsFile = GetWorkingDirectory(conf, c.TestsFile)
	}

	enabled := c.isTestSelectionAllowed(conf)
	trace.SpanFromContext(ctx).SetAttributes(attribute.Bool(testSelectionEnabledAttribute, enabled))
	trace.SpanFromContext(ctx).SetAttributes(attribute.StringSlice(testSelectionInputTestsAttribute, c.Tests))
	if !enabled {
		logger.Execution().Info("Test selection is not allowed/enabled, writing empty test list")
		return c.writeTestList([]string{})
	}

	// No-op based on usage rate. Use the task's random seed so that it's
	// consistent across multiple runs of the same task.
	if c.rate != 0 {
		rng := rand.New(rand.NewSource(createSeed(conf.Task.Id)))
		// Random float in [0.0, 1.0) will always have a
		// usage_rate percentage chance of no-oping.
		if rng.Float64() < c.rate {
			logger.Execution().Infof("Skipping test selection based on usage rate '%s'", c.UsageRate)
			return c.writeTestList([]string{})
		}
		trace.SpanFromContext(ctx).SetAttributes(attribute.Float64(testSelectionUsageRateAttribute, c.rate))
	}

	if c.TestsFile != "" {
		testsFromFile, err := c.parseTestsFromFile()
		if err != nil {
			return errors.Wrap(err, "parsing tests from file")
		}
		c.Tests = testsFromFile
	}

	// Build the request using task information from TaskConfig.
	request := model.SelectTestsRequest{
		Project:      conf.Task.Project,
		Requester:    conf.Task.Requester,
		BuildVariant: conf.Task.BuildVariant,
		TaskID:       conf.Task.Id,
		TaskName:     conf.Task.DisplayName,
		Tests:        c.Tests,
	}

	if c.Strategies != "" {
		trimmedStrategies := strings.TrimSpace(c.Strategies)
		request.Strategies = strings.Split(trimmedStrategies, ",")
		trace.SpanFromContext(ctx).SetAttributes(attribute.StringSlice(testSelectionStrategiesAttribute, request.Strategies))
	}

	startTime := time.Now()
	selectedTests, err := comm.SelectTests(ctx, conf.TaskData(), request)
	durationMs := time.Since(startTime).Milliseconds()
	calledAPI = true
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int64(testSelectionDurationMsAttribute, durationMs))
	if err != nil {
		return errors.Wrap(err, "calling test selection API")
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Int(testSelectionNumTestsReturnedAttribute, len(selectedTests)))

	// Write the results to the output file.
	return c.writeTestList(selectedTests)
}

// isTestSelectionAllowed checks if test selection is allowed in the project and the running task.
func (c *testSelectionGet) isTestSelectionAllowed(conf *internal.TaskConfig) bool {
	return utility.FromBoolPtr(conf.ProjectRef.TestSelection.Allowed) && conf.Task.TestSelectionEnabled
}

// writeTestList writes the list of tests to the output file as JSON in the required format.
func (c *testSelectionGet) writeTestList(tests []string) error {
	testObjects := make([]testOutput, len(tests))
	for i, testName := range tests {
		testObjects[i] = testOutput{Name: testName}
	}

	output := testSelectionOutputFile{
		Tests: testObjects,
	}

	err := utility.WriteJSONFile(c.OutputFile, output)
	return errors.Wrap(err, "writing test selection output to file")
}

// createSeed creates a seed for the random number generator based on the task ID.
func createSeed(taskID string) int64 {
	h := md5.New()
	_, _ = io.WriteString(h, taskID)
	return int64(binary.BigEndian.Uint64(h.Sum(nil)))
}

// parseTestsFromFile reads the tests from the input test JSON file if any and
// returns the list of parsed tests.
// kim: TODO: add unit test.
// kim: TODO: test in staging.
func (c *testSelectionGet) parseTestsFromFile() ([]string, error) {
	if c.TestsFile == "" {
		return nil, nil
	}

	var output testSelectionInputFile
	if err := utility.ReadJSONFile(c.TestsFile, &output); err != nil {
		return nil, errors.Wrap(err, "reading tests from JSON file")
	}

	return output.Tests, nil
}
