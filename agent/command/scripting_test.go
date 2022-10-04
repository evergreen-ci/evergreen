package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScripting(t *testing.T) {
	t.Run("Constructor", func(t *testing.T) {
		cmd, ok := subprocessScriptingFactory().(*scriptingExec)
		assert.True(t, ok)
		assert.Equal(t, "subprocess.scripting", cmd.Name())
	})
	t.Run("Parse", func(t *testing.T) {
		t.Run("ErrorsWithEmpty", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{})
			assert.Error(t, err)
		})
		t.Run("ErrorMismatchedTypes", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{"args": true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "decoding")
		})
		t.Run("NoArgsErrors", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must specify")
		})
		t.Run("BothArgsAndScript", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{"args": []string{"ls"}, "script": "ls"})
			assert.Error(t, err)
		})
		t.Run("ErrorsForArgsAndTestDir", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{
				"args":     []string{"arg"},
				"test_dir": "dir",
			})
			assert.Error(t, err)
		})
		t.Run("ErrorsForCommandAndTestDir", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{
				"command":  "script",
				"test_dir": "dir",
			})
			assert.Error(t, err)

		})
		t.Run("ErrorsForScriptAndTestDir", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{
				"script":   "script",
				"test_dir": "dir",
			})
			assert.Error(t, err)
		})
		t.Run("ErrorsForTestOptionsWithoutTestDir", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{
				"test_options": map[string]interface{}{
					"name": "name",
				},
			})
			assert.Error(t, err)
		})
		t.Run("SplitCommandWithArgs", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{"args": []string{"ls"}, "command": "ls"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "as either arguments or a command")
		})
		t.Run("SplitCommand", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{"command": "ls"})
			require.NoError(t, err)
			assert.Equal(t, "ls", cmd.Args[0])
			assert.NotNil(t, cmd.Env)
		})
		t.Run("IgnoreAndRedirect", func(t *testing.T) {
			cmd := &scriptingExec{Harness: "lisp"}
			err := cmd.ParseParams(map[string]interface{}{"command": "ls", "ignore_standard_out": true, "redirect_standard_error_to_output": true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot ignore standard out, and redirect")
		})
	})

}
