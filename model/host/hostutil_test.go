package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputOptions(t *testing.T) {
	opts := getSSHOutputOptions()
	require.NotNil(t, opts)
	require.NotNil(t, opts.Output)

	output, ok := opts.Output.(*util.CappedWriter)
	require.True(t, ok)
	require.NotNil(t, output)

	require.NoError(t, opts.Validate())
	assert.Equal(t, opts.GetOutput(), opts.GetError())
}

func TestTeardownCommandOverSSH(t *testing.T) {
	cmd := TearDownCommandOverSSH()
	assert.Equal(t, "chmod +x teardown.sh && sh teardown.sh", cmd)
}
