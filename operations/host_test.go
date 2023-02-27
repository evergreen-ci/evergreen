package operations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostSetupScript(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	dir := t.TempDir()

	// With no setup script, noop
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := runSetupScript(ctx, dir, false)
	assert.NoError(err)

	// With a setup script, run the script
	content := []byte("echo \"hello, world\"")
	err = os.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	defer os.Remove(evergreen.TempSetupScriptName)
	defer os.Remove(evergreen.SetupScriptName)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runSetupScript(ctx, dir, false)
	assert.NoError(err)

	// Ensure the script is deleted after running
	_, err = os.Stat(evergreen.SetupScriptName)
	assert.True(os.IsNotExist(err))
	_, err = os.Stat(evergreen.TempSetupScriptName)
	assert.True(os.IsNotExist(err))

	// Script should time out with context and return an error
	err = os.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	err = runSetupScript(ctx, dir, false)
	assert.Error(err)
	assert.Contains(err.Error(), context.DeadlineExceeded.Error())

	// A non-zero exit status should return an error
	content = []byte("exit 1")
	err = os.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	err = runSetupScript(ctx, dir, false)
	assert.Error(err)

}

func TestHostSudoShHelper(t *testing.T) {
	assert := assert.New(t)

	cmd := host.ShCommandWithSudo("foo", false)
	assert.Equal([]string{"sh", "foo"}, cmd)

	cmd = host.ShCommandWithSudo("foo", true)
	assert.Equal([]string{"sudo", "sh", "foo"}, cmd)
}
