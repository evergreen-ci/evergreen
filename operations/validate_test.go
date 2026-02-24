package operations

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli"
)

func TestGetLocalModulesFromInput(t *testing.T) {
	for testName, testCase := range map[string]struct {
		input     []string
		expected  map[string]string
		expectErr bool
	}{
		"EmptyInput": {
			input:    []string{},
			expected: map[string]string{},
		},
		"SingleValidModule": {
			input:    []string{"myModule=/path/to/module"},
			expected: map[string]string{"myModule": "/path/to/module"},
		},
		"MultipleValidModules": {
			input:    []string{"mod1=/path1", "mod2=/path2"},
			expected: map[string]string{"mod1": "/path1", "mod2": "/path2"},
		},
		"InvalidFormatNoEquals": {
			input:     []string{"invalidModule"},
			expected:  map[string]string{},
			expectErr: true,
		},
		"InvalidFormatMultipleEquals": {
			input:     []string{"a=b=c"},
			expected:  map[string]string{},
			expectErr: true,
		},
		"MixOfValidAndInvalid": {
			input:     []string{"valid=/path", "invalid"},
			expected:  map[string]string{"valid": "/path"},
			expectErr: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			result, err := getLocalModulesFromInput(testCase.input)
			if testCase.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, testCase.expected, result)
		})
	}
}

func TestValidateCommand(t *testing.T) {
	sampleYAML, err := os.ReadFile(filepath.Join("testdata", "sample.yml"))
	require.NoError(t, err)

	tmpDir := t.TempDir()
	validFile := filepath.Join(tmpDir, "valid.yml")
	require.NoError(t, os.WriteFile(validFile, sampleYAML, 0644))

	validDir := filepath.Join(tmpDir, "valid_dir")
	require.NoError(t, os.Mkdir(validDir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "a.yml"), sampleYAML, 0644))
	require.NoError(t, os.WriteFile(filepath.Join(validDir, "b.yml"), sampleYAML, 0644))

	for testName, testCase := range map[string]struct {
		args      []string
		expectErr string
	}{
		"FailsWithMissingPath": {
			args:      []string{"validate"},
			expectErr: "must specify the path",
		},
		"FailsWithNonexistentFile": {
			args:      []string{"validate", "--path", filepath.Join("nonexistent", "file.yml")},
			expectErr: "does not exist",
		},
		"SucceedsWithValidFile": {
			args: []string{"validate", "--path", validFile},
		},
		"SucceedsWithValidDirectory": {
			args: []string{"validate", "--path", validDir},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mockClient = &client.Mock{}

			app := cli.NewApp()
			app.Commands = []cli.Command{Validate()}

			set := flag.NewFlagSet("test", 0)
			require.NoError(t, set.Parse(testCase.args))

			ctx := cli.NewContext(app, set, nil)
			err := Validate().Run(ctx)

			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestValidateFile(t *testing.T) {
	sampleYAML, err := os.ReadFile(filepath.Join("testdata", "sample.yml"))
	require.NoError(t, err)

	for testName, testCase := range map[string]struct {
		useNonexistentPath bool
		validateResult     validator.ValidationErrors
		validateErr        error
		quiet              bool
		errorOnWarnings    bool
		expectErr          string
	}{
		"SucceedsWithValidFileAndNoErrors": {},
		"FailsWithNonexistentFile": {
			useNonexistentPath: true,
			expectErr:          "reading file",
		},
		"ReturnsErrorWhenValidationHasErrors": {
			validateResult: validator.ValidationErrors{
				{Level: validator.Error, Message: "something is wrong"},
			},
			expectErr: "invalid configuration",
		},
		"ReturnsErrorWhenClientValidateFails": {
			validateErr: errors.New("client error"),
			expectErr:   "validating project",
		},
		"SucceedsWithWarningsWhenErrorOnWarningsIsDisabled": {
			validateResult: validator.ValidationErrors{
				{Level: validator.Warning, Message: "a warning"},
			},
		},
		"ReturnsErrorWithWarningsWhenErrorOnWarningsIsEnabled": {
			quiet: true,
			validateResult: validator.ValidationErrors{
				{Level: validator.Warning, Message: "a warning"},
			},
			errorOnWarnings: true,
			expectErr:       "invalid configuration",
		},
		"SucceedsWithNoticesOnly": {
			validateResult: validator.ValidationErrors{
				{Level: validator.Notice, Message: "a notice"},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mockClient = &client.Mock{
				ValidateResult: testCase.validateResult,
				ValidateErr:    testCase.validateErr,
			}

			var path string
			if testCase.useNonexistentPath {
				path = filepath.Join("nonexistent", "file.yml")
			} else {
				path = filepath.Join(t.TempDir(), "project.yml")
				require.NoError(t, os.WriteFile(path, sampleYAML, 0644))
			}
			err := validateFile(&ClientSettings{}, path, testCase.quiet, testCase.errorOnWarnings, nil, "")

			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestLoadProjectYAML(t *testing.T) {
	sampleYAML, err := os.ReadFile(filepath.Join("testdata", "sample.yml"))
	require.NoError(t, err)

	for testName, testCase := range map[string]struct {
		useNonexistentPath bool
		fileContent        []byte
		expectErr          string
	}{
		"SucceedsWithValidFile": {},
		"FailsWithNonexistentFile": {
			useNonexistentPath: true,
			expectErr:          "reading file",
		},
		"ReturnsErrorWhenLocalValidationFails": {
			fileContent: []byte("invalid: [yaml: bad"),
			expectErr:   "invalid configuration",
		},
	} {
		t.Run(testName, func(t *testing.T) {
			var path string
			if testCase.useNonexistentPath {
				path = filepath.Join("nonexistent", "file.yml")
			} else {
				content := sampleYAML
				if testCase.fileContent != nil {
					content = testCase.fileContent
				}
				path = filepath.Join(t.TempDir(), "project.yml")
				require.NoError(t, os.WriteFile(path, content, 0644))
			}
			projectYaml, err := loadProjectYAML(path, false, false, nil, "")

			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
				assert.Nil(t, projectYaml)
				return
			}

			require.NoError(t, err)
			assert.NotEmpty(t, projectYaml)
		})
	}
}

func TestValidateProjectRemotely(t *testing.T) {
	projectYaml := []byte("tasks: []\nbuildvariants: []")

	for testName, testCase := range map[string]struct {
		projectYaml     []byte
		validateResult  validator.ValidationErrors
		validateErr     error
		quiet           bool
		errorOnWarnings bool
		expectErr       string
	}{
		"SucceedsWithNoErrors": {
			projectYaml: projectYaml,
		},
		"ReturnsErrorWhenClientValidateFails": {
			projectYaml: projectYaml,
			validateErr: errors.New("client error"),
			expectErr:   "validating project",
		},
		"ReturnsErrorWhenValidationHasErrors": {
			projectYaml: projectYaml,
			validateResult: validator.ValidationErrors{
				{Level: validator.Error, Message: "something is wrong"},
			},
			expectErr: "invalid configuration",
		},
		"SucceedsWithWarningsWhenErrorOnWarningsIsDisabled": {
			projectYaml: projectYaml,
			validateResult: validator.ValidationErrors{
				{Level: validator.Warning, Message: "a warning"},
			},
		},
		"ReturnsErrorWithWarningsWhenErrorOnWarningsIsEnabled": {
			projectYaml: projectYaml,
			quiet:       true,
			validateResult: validator.ValidationErrors{
				{Level: validator.Warning, Message: "a warning"},
			},
			errorOnWarnings: true,
			expectErr:       "invalid configuration",
		},
		"SucceedsWithNoticesOnly": {
			projectYaml: projectYaml,
			validateResult: validator.ValidationErrors{
				{Level: validator.Notice, Message: "a notice"},
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			mockClient = &client.Mock{
				ValidateResult: testCase.validateResult,
				ValidateErr:    testCase.validateErr,
			}

			path := filepath.Join(t.TempDir(), "project.yml")
			err := validateProjectRemotely(&ClientSettings{}, testCase.projectYaml, path, testCase.quiet, testCase.errorOnWarnings, "")

			if testCase.expectErr != "" {
				assert.ErrorContains(t, err, testCase.expectErr)
				return
			}

			require.NoError(t, err)
		})
	}
}

func TestLoadProjectIntoWithValidation(t *testing.T) {
	sampleYAML, err := os.ReadFile(filepath.Join("testdata", "sample.yml"))
	require.NoError(t, err)

	errLevel := validator.Error
	for testName, testCase := range map[string]struct {
		data            []byte
		errorOnWarnings bool
		expectPP        bool
		expectErrLevel  *validator.ValidationErrorLevel
	}{
		"SucceedsWithValidYAML": {
			data:     sampleYAML,
			expectPP: true,
		},
		"ReturnsErrorForInvalidYAML": {
			data:           []byte("invalid: [yaml: bad"),
			expectErrLevel: &errLevel,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			opts := &model.GetProjectOpts{
				ReadFileFrom: model.ReadFromLocal,
			}
			pp, _, validationErrs := loadProjectIntoWithValidation(t.Context(), testCase.data, opts, testCase.errorOnWarnings, &model.Project{}, "")

			if testCase.expectErrLevel != nil {
				assert.True(t, validationErrs.Has(*testCase.expectErrLevel))
			} else {
				assert.False(t, validationErrs.Has(validator.Error))
			}

			if testCase.expectPP {
				assert.NotNil(t, pp)
			}
		})
	}
}
