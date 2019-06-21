package host

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/credentials"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/mongodb/jasper/rpc"
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

func newMockCredentials() (*rpc.Credentials, error) {
	return rpc.NewCredentials([]byte("foo"), []byte("bar"), []byte("bat"))
}

func TestJasperCommands(t *testing.T) {
	for opName, opCase := range map[string]func(t *testing.T, h *Host, config evergreen.HostJasperConfig){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := []string{
				"cd \"/foo\"",
				"curl -LO 'www.example.com/download_file-linux-amd64-abc123.tar.gz'",
				"tar xzf 'download_file-linux-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli'",
				"rm -f 'download_file-linux-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(config)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := h.fetchJasperCommands(config)
			cmds := h.FetchJasperCommand(config)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"FetchJasperCommandWithPath": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			path := "/bar"
			expectedCmds := h.fetchJasperCommands(config)
			for i := range expectedCmds {
				expectedCmds[i] = fmt.Sprintf("PATH=%s ", path) + expectedCmds[i]
			}
			cmds := h.FetchJasperCommandWithPath(config, path)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScript": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := []string{h.FetchJasperCommand(config), h.ForceReinstallJasperCommand(config)}
			creds, err := newMockCredentials()
			require.NoError(t, err)
			script, err := h.BootstrapScript(config, creds)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(script, "#!/bin/bash"))
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, script, expectedCmd)
			}
		},
		"ForceReinstallJasperCommand": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			cmd := h.ForceReinstallJasperCommand(config)
			assert.Equal(t, "sudo /foo/jasper_cli jasper service force-reinstall rpc --port=12345 --creds_path=/bar", cmd)
		},
		// "": func(t *testing.T, h *Host, config evergreen.JasperConfig) {},
	} {
		t.Run(opName, func(t *testing.T) {
			h := &Host{Distro: distro.Distro{
				Arch:                  distro.ArchLinuxAmd64,
				CuratorDir:            "/foo",
				JasperCredentialsPath: "/bar",
			}}
			config := evergreen.HostJasperConfig{
				BinaryName:       "jasper_cli",
				DownloadFileName: "download_file",
				URL:              "www.example.com",
				Version:          "abc123",
				Port:             12345,
			}
			opCase(t, h, config)
		})
	}
}

func TestJasperCommandsWindows(t *testing.T) {
	for opName, opCase := range map[string]func(t *testing.T, h *Host, config evergreen.HostJasperConfig){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := []string{
				"cd \"/foo\"",
				"curl -LO 'www.example.com/download_file-windows-amd64-abc123.tar.gz'",
				"tar xzf 'download_file-windows-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli.exe'",
				"rm -f 'download_file-windows-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(config)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := h.fetchJasperCommands(config)
			cmds := h.FetchJasperCommand(config)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"FetchJasperCommandWithPath": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			path := "/bar"
			expectedCmds := h.fetchJasperCommands(config)
			for i := range expectedCmds {
				expectedCmds[i] = fmt.Sprintf("PATH=%s ", path) + expectedCmds[i]
			}
			cmds := h.FetchJasperCommandWithPath(config, path)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScript": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := h.fetchJasperCommands(config)
			path := "/bin"
			for i := range expectedCmds {
				expectedCmds[i] = strings.Replace(fmt.Sprintf("PATH=%s ", path)+expectedCmds[i], "'", "''", -1)
			}
			expectedCmds = append(expectedCmds, h.ForceReinstallJasperCommand(config))
			creds, err := newMockCredentials()
			require.NoError(t, err)
			script, err := h.BootstrapScript(config, creds)
			require.NoError(t, err)
			assert.True(t, strings.HasPrefix(script, "<powershell>"))
			assert.True(t, strings.HasSuffix(script, "</powershell>"))
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, script, expectedCmd)
			}
		},
		"ForceReinstallJasperCommand": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			cmd := h.ForceReinstallJasperCommand(config)
			assert.Equal(t, "/foo/jasper_cli.exe jasper service force-reinstall rpc --port=12345 --creds_path=/bar", cmd)
		},
		"WriteJasperCredentialsFileCommand": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			creds, err := newMockCredentials()
			require.NoError(t, err)
			path := h.Distro.JasperCredentialsPath

			for testName, testCase := range map[string]func(t *testing.T){
				"WithoutJasperCredentialsPath": func(t *testing.T) {
					h.Distro.JasperCredentialsPath = ""
					_, err := h.writeJasperCredentialsFileCommand(config, creds)
					assert.Error(t, err)
				},
				"WithJasperCredentialsPath": func(t *testing.T) {
					cmd, err := h.writeJasperCredentialsFileCommand(config, creds)
					require.NoError(t, err)

					expectedCreds, err := creds.Export()
					require.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("cat > /bar <<EOF\n%s\nEOF", expectedCreds), cmd)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					h.Distro.JasperCredentialsPath = path
					require.NoError(t, db.ClearCollections(credentials.Collection))
					defer func() {
						assert.NoError(t, db.ClearCollections(credentials.Collection))
					}()
					testCase(t)
				})
			}
		},
		"WriteJasperCredentialsFileCommandNoDistroPath": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			creds, err := rpc.NewCredentials([]byte("foo"), []byte("bar"), []byte("bat"))
			require.NoError(t, err)

			h.Distro.JasperCredentialsPath = ""
			_, err = h.writeJasperCredentialsFileCommand(config, creds)
			assert.Error(t, err)
		},
	} {
		t.Run(opName, func(t *testing.T) {
			h := &Host{Distro: distro.Distro{
				Arch:                  distro.ArchWindowsAmd64,
				CuratorDir:            "/foo",
				JasperCredentialsPath: "/bar",
			}}
			config := evergreen.HostJasperConfig{
				BinaryName:       "jasper_cli",
				DownloadFileName: "download_file",
				URL:              "www.example.com",
				Version:          "abc123",
				Port:             12345,
			}
			opCase(t, h, config)
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
