package command

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"io"
	"math/rand"
	"path/filepath"
	"strconv"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type TestOutput struct {
	Name string `json:"name"`
}

type TestSelectionOutput struct {
	Tests []TestOutput `json:"tests"`
}

type testSelectionGet struct {
	// OutputFile is the path to where the JSON file should be written.
	// Required.
	OutputFile string `mapstructure:"output_file" plugin:"expand"`

	// Tests is a list of test names to pass into the TSS API.
	// Optional.
	Tests []string `mapstructure:"tests" plugin:"expand"`

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
	return catcher.Resolve()
}

func (c *testSelectionGet) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
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

	if !c.isTestSelectionAllowed(conf) {
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

	selectedTests, err := comm.SelectTests(ctx, conf.TaskData(), request)
	if err != nil {
		return errors.Wrap(err, "calling test selection API")
	}

	// Write the results to the output file.
	return c.writeTestList(selectedTests)
}

// isTestSelectionAllowed checks if test selection is allowed in the project and the running task.
func (c *testSelectionGet) isTestSelectionAllowed(conf *internal.TaskConfig) bool {
	return utility.FromBoolPtr(conf.ProjectRef.TestSelection.Allowed) && conf.Task.TestSelectionEnabled
}

// writeTestList writes the list of tests to the output file as JSON in the required format.
func (c *testSelectionGet) writeTestList(tests []string) error {
	testObjects := make([]TestOutput, len(tests))
	for i, testName := range tests {
		testObjects[i] = TestOutput{Name: testName}
	}

	output := TestSelectionOutput{
		Tests: testObjects,
	}

	err := utility.WriteJSONFile(c.OutputFile, output)
	return errors.Wrap(err, "writing test selection output to file")
}

// createSeed creates a seed for the random number generator based on the task ID.
func createSeed(taskID string) int64 {
	h := md5.New()
	io.WriteString(h, taskID)
	return int64(binary.BigEndian.Uint64(h.Sum(nil)))
}
