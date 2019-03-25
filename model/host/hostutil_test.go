package host

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
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

func TestCurlCommand(t *testing.T) {
	assert := assert.New(t)
	h := &Host{Distro: distro.Distro{Arch: "windows_amd64"}}
	url := "www.example.com"
	expected := "cd ~ && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' && chmod +x evergreen.exe"
	assert.Equal(expected, h.CurlCommand(url))

	h = &Host{Distro: distro.Distro{Arch: "linux_amd64"}}
	expected = "cd ~ && if [ -f evergreen ]; then ./evergreen get-update --install --force; else curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen; fi"
	assert.Equal(expected, h.CurlCommand(url))
}
