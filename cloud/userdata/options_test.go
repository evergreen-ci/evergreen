package userdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOptionsValidate(t *testing.T) {
	for testName, testCase := range map[string]struct {
		opts        Options
		expectError bool
	}{
		"PassesWithValidDirectiveAndContent": {
			opts: Options{
				Directive: ShellScript,
				Content:   "echo foo",
			},
			expectError: false,
		},
		"PassesWithValidDirectiveAndClosingTagForWindows": {
			opts: Options{
				Directive:  PowerShellScript,
				Content:    "echo foo",
				ClosingTag: PowerShellScriptClosingTag,
			},
			expectError: false,
		},
		"PassesWithValidDirectiveAndClosingTagWhenPersistedForWindows": {
			opts: Options{
				Directive:  PowerShellScript,
				Content:    "echo foo",
				ClosingTag: PowerShellScriptClosingTag,
				Persist:    true,
			},
			expectError: false,
		},
		"FailsForMismatchedClosingTagForWindows": {
			opts: Options{
				Directive:  PowerShellScript,
				Content:    "echo foo",
				ClosingTag: BatchScriptClosingTag,
				Persist:    true,
			},
			expectError: true,
		},
		"FailsForUnnecessaryClosingTag": {
			opts: Options{
				Directive:  ShellScript,
				Content:    "echo foo",
				ClosingTag: PowerShellScriptClosingTag,
			},
			expectError: true,
		},
		"FailsForMissingDirective": {
			opts: Options{
				Content: "echo foo",
			},
			expectError: true,
		},
		"PassesForEmptyContentWithDirective": {
			opts: Options{
				Directive: ShellScript,
			},
			expectError: false,
		},
		"PassesForEmptyContentWithDirectiveAndClosingTag": {
			opts: Options{
				Directive:  PowerShellScript,
				ClosingTag: PowerShellScriptClosingTag,
			},
			expectError: false,
		},
		"FailsForEmpty": {
			opts:        Options{},
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			err := testCase.opts.ValidateAndDefault()
			if testCase.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}

	for testName, testCase := range map[string]struct {
		opts         Options
		expectedOpts Options
	}{
		"DefaultsClosingTagForValidDirectiveAndUnspecifiedClosingTagForWindows": {
			opts: Options{
				Directive: PowerShellScript,
				Content:   "echo foo",
				Persist:   true,
			},
			expectedOpts: Options{
				Directive:  PowerShellScript,
				Content:    "echo foo",
				ClosingTag: PowerShellScriptClosingTag,
				Persist:    true,
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			err := testCase.opts.ValidateAndDefault()
			assert.NoError(t, err)
			assert.Equal(t, testCase.expectedOpts, testCase.opts)
		})
	}

}
