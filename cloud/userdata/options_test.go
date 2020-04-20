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
		"PassesWithValidDirectivePrefixAndContent": {
			opts: Options{
				Directive: ShellScript + "/bin/bash",
			},
		},
		"PassesWithValidDirectiveForWindows": {
			opts: Options{
				Directive: PowerShellScript,
				Content:   "echo foo",
			},
			expectError: false,
		},
		"PassesWithValidDirectiveWhenPersistedForWindows": {
			opts: Options{
				Directive: PowerShellScript,
				Content:   "echo foo",
				Persist:   true,
			},
			expectError: false,
		},
		"FailsForDirectiveNotPersistableForWindows": {
			opts: Options{
				Directive: CloudBoothook,
				Content:   "echo foo",
				Persist:   true,
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
		"FailsForEmpty": {
			opts:        Options{},
			expectError: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			err := testCase.opts.Validate()
			if !testCase.expectError {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
