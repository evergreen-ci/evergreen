package operations

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strings"
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

	dir, err := ioutil.TempDir("", "")
	require.NoError(err)
	defer os.RemoveAll(dir)

	// With no setup script, noop
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = runSetupScript(ctx, dir, false)
	assert.NoError(err)

	// With a setup script, run the script
	content := []byte("echo \"hello, world\"")
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
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
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	err = runSetupScript(ctx, dir, false)
	assert.Error(err)
	assert.Contains(err.Error(), "context deadline exceeded")

	// A non-zero exit status should return an error
	content = []byte("exit 1")
	err = ioutil.WriteFile(evergreen.SetupScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	err = runSetupScript(ctx, dir, false)
	assert.Error(err)

}

func TestHostSudoShHelper(t *testing.T) {
	assert := assert.New(t)

	cmd := host.ShCommandWithSudo(context.Background(), "foo", false)
	assert.Equal([]string{"sh", "foo"}, cmd.Args)

	cmd = host.ShCommandWithSudo(context.Background(), "foo", true)
	assert.Equal([]string{"sudo", "sh", "foo"}, cmd.Args)
}

func TestHostTeardownScript(t *testing.T) {
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
	err = runHostTeardownScript(ctx)
	assert.NoError(err)

	// Script should time out with context and return an error
	err = ioutil.WriteFile(evergreen.TeardownScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	time.Sleep(time.Millisecond)
	err = runHostTeardownScript(ctx)
	assert.Error(err)
	assert.Contains(err.Error(), "context deadline exceeded")

	// A non-zero exit status should return an error
	content = []byte("exit 1")
	err = ioutil.WriteFile(evergreen.TeardownScriptName, content, 0644)
	require.NoError(err)
	ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	err = runHostTeardownScript(ctx)
	assert.Error(err)

}

func TestMakeAWSTags(t *testing.T) {
	assert := assert.New(t)

	tagSlice := []string{"key1=value1", "key2=value2"}
	tags, err := makeAWSTags(tagSlice)
	assert.Equal(tags, map[string]string{
		"key1": "value1",
		"key2": "value2",
	})
	assert.NoError(err)

	// Problem parsing flag
	tagSlice = []string{"key1=value1", "incorrect"}
	tags, err = makeAWSTags(tagSlice)
	assert.EqualError(err, "problem parsing tag 'incorrect'")

	// Key too long
	badKey := strings.Repeat("a", 129)
	tagSlice = []string{"key1=value", fmt.Sprintf("%s=value2", badKey)}
	tags, err = makeAWSTags(tagSlice)
	assert.EqualError(err, fmt.Sprintf("key '%s' is longer than 128 characters", badKey))

	// Value too long
	badValue := strings.Repeat("a", 257)
	tagSlice = []string{"key1=value2", fmt.Sprintf("key2=%s", badValue)}
	tags, err = makeAWSTags(tagSlice)
	assert.EqualError(err, fmt.Sprintf("value '%s' is longer than 256 characters", badValue))

	// Reserved prefix used
	badPrefix := "aws:"
	tagSlice = []string{"key1=value1", fmt.Sprintf("%skey2=value2", badPrefix)}
	tags, err = makeAWSTags(tagSlice)
	assert.EqualError(err, fmt.Sprintf("illegal tag prefix '%s'", badPrefix))
}
