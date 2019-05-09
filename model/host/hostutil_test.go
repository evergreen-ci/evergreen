package host

import (
	"context"
	"fmt"
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

func TestJasperCommands(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := []string{
				"cd \"/foo\"",
				"curl -LO 'www.example.com/download_file-linux-amd64-abc123.tar.gz'",
				"tar xzf 'download_file-linux-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli'",
				"rm -f 'download_file-linux-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(config, dir)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := h.fetchJasperCommands(config, dir)
			cmds := h.FetchJasperCommand(config, dir)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"FetchJasperCommandWithPath": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			path := "/bar"
			expectedCmds := h.fetchJasperCommands(config, dir)
			for i := range expectedCmds {
				expectedCmds[i] = "PATH=/bar " + expectedCmds[i]
			}
			cmds := h.FetchJasperCommandWithPath(config, dir, path)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScript": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := h.fetchJasperCommands(config, dir)
			script := h.BootstrapScript(config, dir)
			assert.True(t, strings.HasPrefix(script, "#!/bin/bash"))
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, script, expectedCmd)
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			h := &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}}
			config := evergreen.JasperConfig{
				BinaryName:       "jasper_cli",
				DownloadFileName: "download_file",
				URL:              "www.example.com",
				Version:          "abc123",
			}
			dir := "/foo"
			testCase(t, h, config, dir)
		})
	}
}

func TestJasperCommandsWindows(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := []string{
				"cd \"/foo\"",
				"curl -LO 'www.example.com/download_file-windows-amd64-abc123.tar.gz'",
				"tar xzf 'download_file-windows-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli.exe'",
				"rm -f 'download_file-windows-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(config, dir)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := h.fetchJasperCommands(config, dir)
			cmds := h.FetchJasperCommand(config, dir)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"FetchJasperCommandWithPath": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := h.fetchJasperCommands(config, dir)
			path := windowsPath
			for i := range expectedCmds {
				expectedCmds[i] = fmt.Sprintf("PATH=%s ", path) + expectedCmds[i]
			}
			cmds := h.FetchJasperCommandWithPath(config, dir, path)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScript": func(t *testing.T, h *Host, config evergreen.JasperConfig, dir string) {
			expectedCmds := h.fetchJasperCommands(config, dir)
			path := windowsPath
			for i := range expectedCmds {
				expectedCmds[i] = strings.Replace(fmt.Sprintf("PATH=%s ", path)+expectedCmds[i], "'", "''", -1)
			}
			script := h.BootstrapScript(config, dir)
			assert.True(t, strings.HasPrefix(script, "<powershell>"))
			assert.True(t, strings.HasSuffix(script, "</powershell>"))
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, script, expectedCmd)
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
			config := evergreen.JasperConfig{
				BinaryName:       "jasper_cli",
				DownloadFileName: "download_file",
				URL:              "www.example.com",
				Version:          "abc123",
			}
			dir := "/foo"
			testCase(t, h, config, dir)
		})
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
