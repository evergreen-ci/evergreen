package cli

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHostSetupScript(t *testing.T) {
	c := HostSetupCommand{}
	assert := assert.New(t)
	require := require.New(t)

	dir, err := ioutil.TempDir("", "")
	require.NoError(err)
	defer os.RemoveAll(dir)

	// With no setup script, noop
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := c.runSetupScript(ctx)
	assert.NoError(err)
	assert.Empty(out)

	// With a setup script, run the script
	content := []byte("echo \"hello, world\"")
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	defer os.Remove(evergreen.SetupScriptName)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err = c.runSetupScript(ctx)
	assert.Equal("hello, world", strings.TrimSpace(out))
	assert.NoError(err)

	// Ensure the script is deleted after running
	_, err = os.Stat(evergreen.SetupScriptName)
	assert.True(os.IsNotExist(err))

	// Script should time out with context and return an error
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	out, err = c.runSetupScript(ctx)
	assert.Equal("", strings.TrimSpace(out))
	assert.Error(err)
	assert.Contains(err.Error(), "context deadline exceeded")

	// A non-zero exit status should return an error
	content = []byte("exit 1")
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	out, err = c.runSetupScript(ctx)
	assert.Equal("", strings.TrimSpace(out))
	assert.Error(err)

}

func TestHostSudoShHelper(t *testing.T) {
	c := HostSetupCommand{}
	assert := assert.New(t) // nolint

	cmd := getShCommandWithSudo(context.Background(), "foo", c.SetupAsSudo)
	assert.Equal([]string{"sh", "foo"}, cmd.Args)
	c.SetupAsSudo = true
	cmd = getShCommandWithSudo(context.Background(), "foo", c.SetupAsSudo)
	assert.Equal([]string{"sudo", "sh", "foo"}, cmd.Args)
}

func TestHostTeardownScript(t *testing.T) {
	c := HostTeardownCommand{}
	assert := assert.New(t)
	require := require.New(t)

	dir, err := ioutil.TempDir("", "")
	require.NoError(err)
	defer os.RemoveAll(dir)

	// With a teardown script, run the script
	content := []byte("echo \"hello, world\"")
	err = ioutil.WriteFile(evergreen.TeardownScriptName, content, 0644)
	require.NoError(err)
	defer os.Remove(evergreen.TeardownScriptName)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := c.runTeardownScript(ctx)
	assert.Equal("hello, world", strings.TrimSpace(out))
	assert.NoError(err)

	// Script should time out with context and return an error
	err = ioutil.WriteFile(evergreen.TeardownScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	out, err = c.runTeardownScript(ctx)
	assert.Equal("", strings.TrimSpace(out))
	assert.Error(err)
	assert.Contains(err.Error(), "context deadline exceeded")

	// A non-zero exit status should return an error
	content = []byte("exit 1")
	err = ioutil.WriteFile(evergreen.TeardownScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	out, err = c.runTeardownScript(ctx)
	assert.Equal("", strings.TrimSpace(out))
	assert.Error(err)

}
