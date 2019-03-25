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
	stdout, err := opts.GetOutput()
	require.NoError(t, err)
	stderr, err := opts.GetError()
	require.NoError(t, err)
	assert.Equal(t, stdout, stderr)
}
