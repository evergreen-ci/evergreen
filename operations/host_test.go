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
	t.Run("OK", func(t *testing.T) {
		tagSlice := []string{"key1=value1", "key2=value2", "key1=value3"}
		tags, err := makeAWSTags(tagSlice)
		assert.Equal(t, tags, []host.Tag{
			host.Tag{
				Key:           "key1",
				Value:         "value3",
				CanBeModified: true,
			},
			host.Tag{
				Key:           "key2",
				Value:         "value2",
				CanBeModified: true,
			},
		})
	})
	t.Run("ParsingError", func(t *testing.T) {
		badTag := "incorrect"
		tagSlice := []string{"key1=value1", badTag}
		tags, err := makeAWSTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("problem parsing tag '%s'", badTag))
	})
	t.Run("LongKey", func(t *testing.T) {
		badKey := strings.Repeat("a", 129)
		tagSlice := []string{"key1=value", fmt.Sprintf("%s=value2", badKey)}
		tags, err := makeAWSTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("key '%s' is longer than 128 characters", badKey))
	})
	t.Run("LongValue", func(t *testing.T) {
		badValue := strings.Repeat("a", 257)
		tagSlice := []string{"key1=value2", fmt.Sprintf("key2=%s", badValue)}
		tags, err := makeAWSTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("value '%s' is longer than 256 characters", badValue))
	})
	t.Run("BadPrefix", func(t *testing.T) {
		badPrefix := "aws:"
		tagSlice := []string{"key1=value1", fmt.Sprintf("%skey2=value2", badPrefix)}
		tags, err := makeAWSTags(tagSlice)
		assert.Nil(t, tags)
		assert.EqualError(t, err, fmt.Sprintf("illegal tag prefix '%s'", badPrefix))
	})
}
