package host

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommand(t *testing.T) {
	assert := assert.New(t)
	h := &Host{Distro: distro.Distro{Arch: "windows_amd64"}}
	url := "www.example.com"
	expected := "cd ~ && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' && chmod +x evergreen.exe"
	assert.Equal(expected, h.CurlCommand(url))

	h = &Host{Distro: distro.Distro{Arch: "linux_amd64"}}
	// expected = "cd ~ && if [ -f evergreen ]; then ./evergreen get-update --install --force; else curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen; fi"
	expected = "cd ~ && curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen"
	assert.Equal(expected, h.CurlCommand(url))
}

func TestTeardownCommandOverSSH(t *testing.T) {
	cmd := TearDownCommandOverSSH()
	assert.Equal(t, "chmod +x teardown.sh && sh teardown.sh", cmd)
}

func TestInitSystemCommand(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("init system test is relevant to Linux only")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", initSystemCommand())
	res, err := cmd.Output()
	require.NoError(t, err)
	initSystem := strings.TrimSpace(string(res))
	assert.Contains(t, []string{InitSystemSystemd, InitSystemSysV, InitSystemUpstart}, initSystem)
}
