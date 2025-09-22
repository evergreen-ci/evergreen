package command

import (
	"context"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TestSelectionOutput represents the output JSON structure
type TestSelectionOutput struct {
	Tests []map[string]string `json:"tests"`
}

type testSelectionGet struct {
	// OutputFile is the path to where the JSON file should be written.
	// Required.
	OutputFile string `mapstructure:"output_file" plugin:"expand"`

	// Tests is a list of test names to pass into the TSS API.
	// Optional.
	Tests []string `mapstructure:"tests" plugin:"expand"`

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

	return catcher.Resolve()
}

func (c *testSelectionGet) Execute(ctx context.Context, comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {
	if err := util.ExpandValues(c, &conf.Expansions); err != nil {
		return errors.Wrap(err, "applying expansions")
	}

	// Re-validate the command here, in case an expansion is not defined.
	if err := c.validate(); err != nil {
		return errors.WithStack(err)
	}

	// Resolve the output file path early so it's available for writing empty results
	if !filepath.IsAbs(c.OutputFile) {
		c.OutputFile = GetWorkingDirectory(conf, c.OutputFile)
	}

	if !c.isTestSelectionAllowed(conf) {
		logger.Execution().Info("Test selection is not allowed/enabled, writing empty test list")
		return c.writeTestList([]string{})
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

// isTestSelectionAllowed checks if test selection is allowed in the project and the running task
func (c *testSelectionGet) isTestSelectionAllowed(conf *internal.TaskConfig) bool {
	return utility.FromBoolPtr(conf.ProjectRef.TestSelection.Allowed) && conf.Task.TestSelectionEnabled
}

// writeTestList writes the list of tests to the output file as JSON in the required format
func (c *testSelectionGet) writeTestList(tests []string) error {
	testObjects := make([]map[string]string, len(tests))
	for i, testName := range tests {
		testObjects[i] = map[string]string{"name": testName}
	}

	output := TestSelectionOutput{
		Tests: testObjects,
	}

	err := utility.WriteJSONFile(c.OutputFile, output)
	if err != nil {
		return errors.Wrap(err, "writing test selection output to file")
	}
	return nil
}
