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
		t.Run("ErrorMisMatchedTypes", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{"args": true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "error decoding")
		})
		t.Run("NoArgsErrors", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(nil)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "must specify")
		})
		t.Run("BothArgsAndScript", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{"args": []string{"ls"}, "script": "ls"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "but not both")
		})
		t.Run("SplitCommandWithArgs", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{"args": []string{"ls"}, "command": "ls"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "as either arguments or a command")
		})
		t.Run("SplitCommand", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{"command": "ls"})
			require.NoError(t, err)
			assert.Equal(t, "ls", cmd.Args[0])
			assert.NotNil(t, cmd.Env)
		})
		t.Run("IgnoreAndRedirect", func(t *testing.T) {
			cmd := &scriptingExec{}
			err := cmd.ParseParams(map[string]interface{}{"command": "ls", "ignore_standard_out": true, "redirect_standard_error_to_output": true})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot ignore standard out, and redirect")
		})

	})

}
