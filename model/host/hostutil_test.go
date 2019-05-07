package host

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommand(t *testing.T) {
	assert := assert.New(t)
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	url := "www.example.com"
	expected := "cd ~ && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' && chmod +x evergreen.exe"
	assert.Equal(expected, h.CurlCommand(url))

	h = &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}}
	// expected = "cd ~ && if [ -f evergreen ]; then ./evergreen get-update --install --force; else curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen; fi"
	expected = "cd ~ && curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen"
	assert.Equal(expected, h.CurlCommand(url))
}

func TestFetchJasperCommand(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}}
	config := evergreen.JasperConfig{
		BinaryName:       "jasper_cli",
		DownloadFileName: "download_file",
		URL:              "www.example.com",
		Version:          "abc123",
	}
	outDir := "foo"
	expectedParts := []string{
		"cd \"foo\"",
		"curl -LO 'www.example.com/download_file-linux-amd64-abc123.tar.gz'",
		"tar xzf 'download_file-linux-amd64-abc123.tar.gz'",
		"chmod +x 'jasper_cli'",
		"rm -f 'download_file-linux-amd64-abc123.tar.gz'",
	}
	cmd := h.FetchJasperCommand(config, outDir)
	for _, expected := range expectedParts {
		assert.Contains(t, cmd, expected)
	}
}

func TestPowerShellFetchJasperCommand(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	config := evergreen.JasperConfig{
		BinaryName:       "jasper_cli",
		DownloadFileName: "download_file",
		URL:              "www.example.com",
		Version:          "abc123",
	}
	outDir := "/foo/bar"
	expectedParts := []string{
		"cd \"\\foo\\bar\"",
		"Invoke-RestMethod -Method Get -Uri 'www.example.com/download_file-windows-amd64-abc123.tar.gz' -OutFile '\\foo\\bar\\download_file-windows-amd64-abc123.tar.gz'",
		"tar xzf 'download_file-windows-amd64-abc123.tar.gz'",
		"rm -f 'download_file-windows-amd64-abc123.tar.gz'",
	}
	cmd := h.PowerShellFetchJasperCommand(config, outDir)
	for _, expected := range expectedParts {
		assert.Contains(t, cmd, expected)
	}
}

func TestPowerShellFetchJasperCommandWindowsDirectory(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	config := evergreen.JasperConfig{
		BinaryName:       "jasper_cli",
		DownloadFileName: "download_file",
		URL:              "www.example.com",
		Version:          "abc123",
	}
	outDir := "C:\\windows\\style\\dir"
	expectedParts := []string{
		"cd \"C:\\windows\\style\\dir\"",
		"Invoke-RestMethod -Method Get -Uri 'www.example.com/download_file-windows-amd64-abc123.tar.gz' -OutFile '\\foo\\bar\\download_file-windows-amd64-abc123.tar.gz'",
		"tar xzf 'download_file-windows-amd64-abc123.tar.gz'",
		"rm -f 'download_file-windows-amd64-abc123.tar.gz'",
	}
	cmd := h.PowerShellFetchJasperCommand(config, outDir)
	for _, expected := range expectedParts {
		assert.Contains(t, cmd, expected)
	}
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
