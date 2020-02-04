package host

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	jutil "github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommand(t *testing.T) {
	assert := assert.New(t)
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64, User: "user"}}
	settings := &evergreen.Settings{
		Ui:                evergreen.UIConfig{Url: "www.example.com"},
		ClientBinariesDir: "clients",
	}
	expected := "cd /home/user && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' && chmod +x evergreen.exe"
	assert.Equal(expected, h.CurlCommand(settings))

	h = &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64, User: "user"}}
	expected = "cd /home/user && curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen"
	assert.Equal(expected, h.CurlCommand(settings))
}

func TestCurlCommandWithRetry(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64, User: "user"}}
	settings := &evergreen.Settings{
		Ui:                evergreen.UIConfig{Url: "www.example.com"},
		ClientBinariesDir: "clients",
	}
	expected := "cd /home/user && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' --retry 5 --retry-max-time 10 && chmod +x evergreen.exe"
	assert.Equal(t, expected, h.CurlCommandWithRetry(settings, 5, 10))

	h = &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64, User: "user"}}
	expected = "cd /home/user && curl -LO 'www.example.com/clients/linux_amd64/evergreen' --retry 5 --retry-max-time 10 && chmod +x evergreen"
	assert.Equal(t, expected, h.CurlCommandWithRetry(settings, 5, 10))
}

func TestClientURL(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	settings := &evergreen.Settings{
		Ui:                evergreen.UIConfig{Url: "www.example.com"},
		ClientBinariesDir: "clients",
	}

	expected := "www.example.com/clients/windows_amd64/evergreen.exe"
	assert.Equal(t, expected, h.ClientURL(settings))

	h.Distro.Arch = distro.ArchLinuxAmd64
	expected = "www.example.com/clients/linux_amd64/evergreen"
	assert.Equal(t, expected, h.ClientURL(settings))
}

func TestJasperCommands(t *testing.T) {
	for opName, opCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := []string{
				"mkdir -m 777 -p /foo",
				"cd /foo",
				fmt.Sprintf("curl -LO 'www.example.com/download_file-linux-amd64-abc123.tar.gz' --retry %d --retry-max-time %d", curlDefaultNumRetries, curlDefaultMaxSecs),
				"tar xzf 'download_file-linux-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli'",
				"rm -f 'download_file-linux-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(settings.HostJasper)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := h.fetchJasperCommands(settings.HostJasper)
			cmds := h.FetchJasperCommand(settings.HostJasper)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScriptForAgent": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert())

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			startAgentMonitor, err := h.StartAgentMonitorRequest(settings)
			require.NoError(t, err)

			markDone, err := h.MarkUserDataDoneCommands()
			require.NoError(t, err)

			expectedCmds := []string{
				setupScript,
				h.FetchJasperCommand(settings.HostJasper),
				h.ForceReinstallJasperCommand(settings),
				h.CurlCommandWithRetry(settings, curlDefaultNumRetries, curlDefaultMaxSecs),
				startAgentMonitor,
				markDone,
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.BootstrapScript(settings, creds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "#!/bin/bash"))

			currPos := 0
			offset := strings.Index(script[currPos:], setupScript)
			require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", setupScript))
			currPos += offset + len(setupScript)

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}
		},
		"BootstrapScriptForSpawnHost": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			userID := "user"
			user := &user.DBUser{Id: userID}
			require.NoError(t, user.Insert())

			h.ProvisionOptions = &ProvisionOptions{LoadCLI: true, OwnerId: userID}
			require.NoError(t, h.Insert())

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			setupSpawnHost, err := h.SetupSpawnHostCommands(settings)
			require.NoError(t, err)

			markDone, err := h.MarkUserDataDoneCommands()
			require.NoError(t, err)

			expectedCmds := []string{
				setupScript,
				h.FetchJasperCommand(settings.HostJasper),
				h.ForceReinstallJasperCommand(settings),
				h.CurlCommandWithRetry(settings, curlDefaultNumRetries, curlDefaultMaxSecs),
				setupSpawnHost,
				markDone,
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.BootstrapScript(settings, creds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "#!/bin/bash"))

			currPos := 0
			offset := strings.Index(script[currPos:], setupScript)
			require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", setupScript))
			currPos += offset + len(setupScript)

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}
		},
		"ForceReinstallJasperCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.Distro.User))
			assert.Contains(t, cmd, fmt.Sprintf("--env 'envKey0=envValue0'"))
			assert.Contains(t, cmd, fmt.Sprintf("--env 'envKey1=envValue1'"))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_num_procs=%d", h.Distro.BootstrapSettings.ResourceLimits.NumProcesses))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_num_files=%d", h.Distro.BootstrapSettings.ResourceLimits.NumFiles))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_virtual_memory=%d", h.Distro.BootstrapSettings.ResourceLimits.VirtualMemoryKB))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_locked_memory=%d", h.Distro.BootstrapSettings.ResourceLimits.LockedMemoryKB))
		},
		"ForceReinstallJasperCommandWithSplunkLogging": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			settings.Splunk.ServerURL = "url"
			settings.Splunk.Token = "token"
			settings.Splunk.Channel = "channel"

			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))

			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.Distro.User))
			assert.Contains(t, cmd, fmt.Sprintf("--splunk_url=%s", settings.Splunk.ServerURL))
			assert.Contains(t, cmd, fmt.Sprintf("--splunk_token_path=%s", h.splunkTokenFilePath()))
			assert.Contains(t, cmd, fmt.Sprintf("--splunk_channel=%s", settings.Splunk.Channel))
		},
	} {
		t.Run(opName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Distro: distro.Distro{
					Arch: distro.ArchLinuxAmd64,
					BootstrapSettings: distro.BootstrapSettings{
						JasperBinaryDir:       "/foo",
						JasperCredentialsPath: "/bar/bat.txt",
						Env: []distro.EnvVar{
							{Key: "envKey0", Value: "envValue0"},
							{Key: "envKey1", Value: "envValue1"},
						},
						ResourceLimits: distro.ResourceLimits{
							NumProcesses:    1,
							NumFiles:        2,
							LockedMemoryKB:  3,
							VirtualMemoryKB: 4,
						},
					},
					User: "user",
				}}
			settings := &evergreen.Settings{
				HostJasper: evergreen.HostJasperConfig{
					BinaryName:       "jasper_cli",
					DownloadFileName: "download_file",
					URL:              "www.example.com",
					Version:          "abc123",
					Port:             12345,
				},
			}
			opCase(t, h, settings)
		})
	}
}

func TestJasperCommandsWindows(t *testing.T) {
	for opName, opCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := []string{
				"mkdir -m 777 -p /foo",
				"cd /foo",
				fmt.Sprintf("curl -LO 'www.example.com/download_file-windows-amd64-abc123.tar.gz' --retry %d --retry-max-time %d", curlDefaultNumRetries, curlDefaultMaxSecs),
				"tar xzf 'download_file-windows-amd64-abc123.tar.gz'",
				"chmod +x 'jasper_cli.exe'",
				"rm -f 'download_file-windows-amd64-abc123.tar.gz'",
			}
			cmds := h.fetchJasperCommands(settings.HostJasper)
			require.Len(t, cmds, len(expectedCmds))
			for i := range expectedCmds {
				assert.Equal(t, expectedCmds[i], cmds[i])
			}
		},
		"FetchJasperCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := h.fetchJasperCommands(settings.HostJasper)
			cmds := h.FetchJasperCommand(settings.HostJasper)
			for _, expectedCmd := range expectedCmds {
				assert.Contains(t, cmds, expectedCmd)
			}
		},
		"BootstrapScriptForAgent": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert())

			setupUser, err := h.SetupServiceUserCommands()
			require.NoError(t, err)

			writeSetupScript, err := h.writeSetupScriptCommands(settings)
			require.NoError(t, err)

			expectedPreCmds := append([]string{setupUser}, writeSetupScript...)

			creds, err := newMockCredentials()
			require.NoError(t, err)
			writeCredentialsCmd, err := h.bufferedWriteJasperCredentialsFilesCommands(settings.Splunk, creds)
			require.NoError(t, err)

			startAgentMonitor, err := h.StartAgentMonitorRequest(settings)
			require.NoError(t, err)

			markDone, err := h.MarkUserDataDoneCommands()
			require.NoError(t, err)

			expectedCmds := append(writeCredentialsCmd,
				h.FetchJasperCommand(settings.HostJasper),
				h.ForceReinstallJasperCommand(settings),
				h.CurlCommandWithRetry(settings, curlDefaultNumRetries, curlDefaultMaxSecs),
				h.SetupCommand(),
				startAgentMonitor,
				markDone,
			)

			for i := range expectedCmds {
				expectedCmds[i] = util.PowerShellQuotedString(expectedCmds[i])
			}

			script, err := h.BootstrapScript(settings, creds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "<powershell>"))
			assert.True(t, strings.HasSuffix(script, "</powershell>"))

			currPos := 0
			for _, expectedCmd := range expectedPreCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}
		},
		"BootstrapScriptForSpawnHost": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			userID := "user"
			user := &user.DBUser{Id: userID}
			require.NoError(t, user.Insert())
			h.ProvisionOptions = &ProvisionOptions{LoadCLI: true, OwnerId: userID}
			require.NoError(t, h.Insert())

			setupUser, err := h.SetupServiceUserCommands()
			require.NoError(t, err)

			writeSetupScript, err := h.writeSetupScriptCommands(settings)
			require.NoError(t, err)

			expectedPreCmds := append([]string{setupUser}, writeSetupScript...)

			creds, err := newMockCredentials()
			require.NoError(t, err)
			writeCredentialsCmd, err := h.bufferedWriteJasperCredentialsFilesCommands(settings.Splunk, creds)
			require.NoError(t, err)

			setupSpawnHost, err := h.SetupSpawnHostCommands(settings)
			require.NoError(t, err)

			markDone, err := h.MarkUserDataDoneCommands()
			require.NoError(t, err)

			expectedCmds := append(writeCredentialsCmd,
				h.FetchJasperCommand(settings.HostJasper),
				h.ForceReinstallJasperCommand(settings),
				h.CurlCommandWithRetry(settings, curlDefaultNumRetries, curlDefaultMaxSecs),
				h.SetupCommand(),
				setupSpawnHost,
				markDone,
			)

			for i := range expectedCmds {
				expectedCmds[i] = util.PowerShellQuotedString(expectedCmds[i])
			}

			script, err := h.BootstrapScript(settings, creds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "<powershell>"))
			assert.True(t, strings.HasSuffix(script, "</powershell>"))

			currPos := 0
			for _, expectedCmd := range expectedPreCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset, fmt.Sprintf("missing %s", expectedCmd))
				currPos += offset + len(expectedCmd)
			}
		},
		"ForceReinstallJasperCommandSSH": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert())
			require.NoError(t, h.CreateServicePassword())
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "/foo/jasper_cli.exe jasper service force-reinstall rpc"))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf(`--user=.\\%s`, h.Distro.BootstrapSettings.ServiceUser))
			assert.Contains(t, cmd, fmt.Sprintf("--password='%s'", h.ServicePassword))
		},
		"WriteJasperCredentialsFileCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			creds, err := newMockCredentials()
			require.NoError(t, err)

			for testName, testCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
				"WithoutJasperCredentialsPath": func(t *testing.T, h *Host, settings *evergreen.Settings) {
					h.Distro.BootstrapSettings.JasperCredentialsPath = ""
					_, err := h.WriteJasperCredentialsFilesCommands(settings.Splunk, creds)
					assert.Error(t, err)
				},
				"WithJasperCredentialsPath": func(t *testing.T, h *Host, settings *evergreen.Settings) {
					cmd, err := h.WriteJasperCredentialsFilesCommands(settings.Splunk, creds)
					require.NoError(t, err)

					expectedCreds, err := creds.Export()
					require.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("mkdir -m 777 -p /bar && echo '%s' > '/bar/bat.txt'", expectedCreds), cmd)
				},
				"WithSplunkCredentials": func(t *testing.T, h *Host, settings *evergreen.Settings) {
					settings.Splunk.Token = "token"
					settings.Splunk.ServerURL = "splunk_url"
					cmd, err := h.WriteJasperCredentialsFilesCommands(settings.Splunk, creds)
					require.NoError(t, err)

					expectedCreds, err := creds.Export()
					require.NoError(t, err)
					assert.Equal(t, fmt.Sprintf("mkdir -m 777 -p /bar && echo '%s' > '/bar/bat.txt' && echo '%s' > '/bar/splunk.txt'", expectedCreds, settings.Splunk.Token), cmd)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(evergreen.CredentialsCollection))
					defer func() {
						assert.NoError(t, db.ClearCollections(evergreen.CredentialsCollection))
					}()
					hostCopy := *h
					settingsCopy := *settings
					testCase(t, &hostCopy, &settingsCopy)
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{Distro: distro.Distro{
				Arch: distro.ArchWindowsAmd64,
				BootstrapSettings: distro.BootstrapSettings{
					Method:                distro.BootstrapMethodUserData,
					Communication:         distro.CommunicationMethodRPC,
					JasperBinaryDir:       "/foo",
					JasperCredentialsPath: "/bar/bat.txt",
					ShellPath:             "/bin/bash",
					ServiceUser:           "service-user",
				},
				Setup: "#!/bin/bash\necho hello",
				User:  "user",
			}}
			settings := &evergreen.Settings{
				HostJasper: evergreen.HostJasperConfig{
					BinaryName:       "jasper_cli",
					DownloadFileName: "download_file",
					URL:              "www.example.com",
					Version:          "abc123",
					Port:             12345,
				},
			}
			opCase(t, h, settings)
		})
	}
}

func TestJasperClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const (
		sshKeyName  = "foo"
		sshKeyValue = "bar"
	)
	for testName, testCase := range map[string]struct {
		withSetupAndTeardown func(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *Host, fn func()) error
		h                    *Host
		expectError          bool
	}{
		"PassesWithSSHCommunicationAndSSHInfo": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
					SSHKey: sshKeyName,
				},
				User: "foo",
				Host: "bar",
			},
			expectError: false,
		},
		"PassesWithHostReprovisioningToLegacy": {
			h: &Host{
				Id:               "test-host",
				NeedsReprovision: ReprovisionToLegacy,
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
					SSHKey: sshKeyName,
				},
			},
			expectError: true,
		},
		"FailsWithLegacyHost": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodLegacySSH,
						Communication: distro.CommunicationMethodLegacySSH,
					},
					SSHKey: sshKeyName,
				},
			},
			expectError: true,
		},
		"FailsWithSSHCommunicationButNoSSHKey": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
				},
				User: "foo",
				Host: "bar",
			},
			expectError: true,
		},
		"FailsWithSSHCommunicationButNoSSHInfo": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodSSH,
					},
					SSHKey: sshKeyName,
				},
			},
			expectError: true,
		},
		"FailsWithRPCCommunicationButNoNetworkAddress": {
			withSetupAndTeardown: withJasperServiceSetupAndTeardown,
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodRPC,
					},
				},
			},
			expectError: true,
		},
		"FailsWithRPCCommunicationButNoJasperService": {
			withSetupAndTeardown: func(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *Host, fn func()) error {
				if err := errors.WithStack(setupCredentialsCollection(ctx, env)); err != nil {
					grip.Error(errors.WithStack(teardownJasperService(ctx, nil)))
					return errors.WithStack(err)
				}
				fn()
				return nil
			},
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodRPC,
					},
				},
				Host: "localhost",
			},
			expectError: true,
		},
		"PassesWithRPCCommunicationAndHostRunningJasperService": {
			withSetupAndTeardown: withJasperServiceSetupAndTeardown,
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodSSH,
						Communication: distro.CommunicationMethodRPC,
					},
				},
				Host: "localhost",
			},
			expectError: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			env.Settings().HostJasper.BinaryName = "binary"
			env.Settings().Keys = map[string]string{sshKeyName: sshKeyValue}

			doTest := func() {
				client, err := testCase.h.JasperClient(tctx, env)
				defer func() {
					if client != nil {
						assert.NoError(t, client.CloseConnection())
					}
				}()
				if testCase.expectError {
					assert.Error(t, err)
					assert.Nil(t, client)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, client)
				}
			}

			if testCase.withSetupAndTeardown != nil {
				assert.NoError(t, testCase.withSetupAndTeardown(tctx, env, &jmock.Manager{}, testCase.h, doTest))
			} else {
				doTest()
			}
		})
	}
}

func TestJasperProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create){
		"RunJasperProcessPasses": func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create) {
			assert.NoError(t, withJasperServiceSetupAndTeardown(ctx, env, manager, h, func() {
				_, err := h.RunJasperProcess(ctx, env, opts)
				assert.NoError(t, err)
			}))
		},
		"StartJasperProcessPasses": func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create) {
			assert.NoError(t, withJasperServiceSetupAndTeardown(ctx, env, manager, h, func() {
				_, err := h.StartJasperProcess(ctx, env, opts)
				assert.NoError(t, err)
			}))
		},
		"RunJasperProcessFailsIfProcessCreationFails": func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create) {
			manager.FailCreate = true
			assert.NoError(t, withJasperServiceSetupAndTeardown(ctx, env, manager, h, func() {
				_, err := h.RunJasperProcess(ctx, env, opts)
				assert.Error(t, err)
			}))
		},
		"RunJasperProcessFailsIfProcessExitsWithError": func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create) {
			manager.Create = func(*options.Create) jmock.Process {
				return jmock.Process{FailWait: true}
			}
			assert.NoError(t, withJasperServiceSetupAndTeardown(ctx, env, manager, h, func() {
				_, err := h.RunJasperProcess(ctx, env, opts)
				assert.Error(t, err)
			}))
		},
		"StartJasperProcessFailsIfProcessCreationFails": func(ctx context.Context, t *testing.T, env *mock.Environment, manager *jmock.Manager, h *Host, opts *options.Create) {
			manager.FailCreate = true
			assert.NoError(t, withJasperServiceSetupAndTeardown(ctx, env, manager, h, func() {
				_, err := h.StartJasperProcess(ctx, env, opts)
				assert.Error(t, err)
			}))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			env.Settings().HostJasper.BinaryName = "binary"

			manager := &jmock.Manager{}
			env.JasperProcessManager = manager

			opts := &options.Create{Args: []string{"echo", "hello", "world"}}

			testCase(tctx, t, env, manager, &Host{
				Id: "test",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
				},
				Host: "localhost",
			}, opts)
		})
	}
}

func TestBufferedWriteFileCommands(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, path, content string){
		"BufferedWriteFileCommandsUsesSingleCommandForShortFile": func(t *testing.T, path, content string) {
			cmds := bufferedWriteFileCommands(path, content)
			require.Len(t, cmds, 2)
			assert.Equal(t, fmt.Sprintf("mkdir -m 777 -p %s", filepath.Dir(path)), cmds[0])
			assert.Equal(t, writeToFileCommand(path, content, true), cmds[1])
		},
		"BufferedWriteFileCommandsUsesMultipleCommandsForLongFile": func(t *testing.T, path, content string) {
			content = strings.Repeat(content, 1000)
			cmds := bufferedWriteFileCommands(path, content)
			require.NotZero(t, len(cmds))
		},
		"WriteToFileCommandRedirectsOnOverwrite": func(t *testing.T, path, content string) {
			cmd := writeToFileCommand(path, content, true)
			assert.Equal(t, fmt.Sprintf("echo -n '%s' > %s", content, path), cmd)
		},
		"WriteToFileCommandAppendsOnNoOverwrite": func(t *testing.T, path, content string) {
			cmd := writeToFileCommand(path, content, false)
			assert.Equal(t, fmt.Sprintf("echo -n '%s' >> %s", content, path), cmd)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, "/path/to/file", "foobar")
		})
	}
}

func TestTearDownDirectlyCommand(t *testing.T) {
	cmd := TearDownDirectlyCommand()
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

func TestBuildLocalJasperClientRequest(t *testing.T) {
	h := &Host{
		Distro: distro.Distro{
			BootstrapSettings: distro.BootstrapSettings{
				JasperBinaryDir:       "/curator",
				JasperCredentialsPath: "/jasper/credentials.txt",
			},
		},
	}

	config := evergreen.HostJasperConfig{Port: 12345, BinaryName: "binary"}
	input := struct {
		Field string `json:"field"`
	}{Field: "foo"}
	subCmd := "sub"

	config.BinaryName = "binary"
	binaryName := h.JasperBinaryFilePath(config)

	cmd, err := h.buildLocalJasperClientRequest(config, subCmd, input)
	require.NoError(t, err)

	index := strings.Index(cmd, binaryName)
	require.NotEqual(t, -1, index)
	index += len(binaryName)

	expectedSubCmd := "jasper client sub"
	offset := strings.Index(cmd[index:], expectedSubCmd)
	require.NotEqual(t, -1, offset)
	index += offset + len(expectedSubCmd)

	assert.Contains(t, cmd[index:], "--service=rpc")
	assert.Contains(t, cmd[index:], fmt.Sprintf("--port=%d", config.Port))
	assert.Contains(t, cmd[index:], fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
}

func TestStartAgentMonitorRequest(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	h := &Host{Id: "id", Distro: distro.Distro{WorkDir: "/foo"}}
	require.NoError(t, h.Insert())

	settings := &evergreen.Settings{
		ApiUrl:            "www.example0.com",
		ClientBinariesDir: "dir",
		LoggerConfig: evergreen.LoggerConfig{
			LogkeeperURL: "www.example1.com",
		},
		Ui: evergreen.UIConfig{
			Url: "www.example2.com",
		},
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				S3Key:    "key",
				S3Secret: "secret",
				Bucket:   "bucket",
			},
		},
		Splunk: send.SplunkConnectionInfo{
			ServerURL: "www.example3.com",
			Token:     "token",
		},
	}

	cmd, err := h.StartAgentMonitorRequest(settings)
	require.NoError(t, err)

	assert.NotEmpty(t, h.Secret)
	dbHost, err := FindOneId(h.Id)
	require.NoError(t, err)
	assert.Equal(t, h.Secret, dbHost.Secret)

	expectedCmd, err := json.Marshal(h.AgentMonitorOptions(settings))
	require.NoError(t, err)
	assert.Contains(t, cmd, string(expectedCmd))

	expectedEnv, err := json.Marshal(buildAgentEnv(settings))
	require.NoError(t, err)
	assert.Contains(t, cmd, string(expectedEnv))
	assert.Contains(t, cmd, evergreen.AgentMonitorTag)
}

func TestStopAgentMonitor(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment, manager *jmock.Manager, h *Host){
		"SendsKillToTaggedRunningProcesses": func(ctx context.Context, t *testing.T, env evergreen.Environment, manager *jmock.Manager, h *Host) {
			proc, err := manager.CreateProcess(ctx, &options.Create{
				Args: []string{"agent", "monitor", "command"},
			})
			require.NoError(t, err)
			proc.Tag(evergreen.AgentMonitorTag)

			mockProc, ok := proc.(*jmock.Process)
			require.True(t, ok)
			mockProc.ProcInfo.IsRunning = true

			require.NoError(t, h.StopAgentMonitor(ctx, env))

			require.Len(t, mockProc.Signals, 1)
			assert.Equal(t, syscall.SIGTERM, mockProc.Signals[0])
		},
		"DoesNotKillProcessesWithoutCorrectTag": func(ctx context.Context, t *testing.T, env evergreen.Environment, manager *jmock.Manager, h *Host) {
			proc, err := manager.CreateProcess(ctx, &options.Create{
				Args: []string{"some", "other", "command"}},
			)
			require.NoError(t, err)

			mockProc, ok := proc.(*jmock.Process)
			require.True(t, ok)
			mockProc.ProcInfo.IsRunning = true

			require.NoError(t, h.StopAgentMonitor(ctx, env))

			assert.Empty(t, mockProc.Signals)
		},
		"DoesNotKillFinishedAgentMonitors": func(ctx context.Context, t *testing.T, env evergreen.Environment, manager *jmock.Manager, h *Host) {
			proc, err := manager.CreateProcess(ctx, &options.Create{
				Args: []string{"agent", "monitor", "command"},
			})
			require.NoError(t, err)
			proc.Tag(evergreen.AgentMonitorTag)

			require.NoError(t, h.StopAgentMonitor(ctx, env))

			mockProc, ok := proc.(*jmock.Process)
			require.True(t, ok)
			assert.Empty(t, mockProc.Signals)
		},
		"NoopsOnLegacyHost": func(ctx context.Context, t *testing.T, env evergreen.Environment, manager *jmock.Manager, h *Host) {
			h.Distro = distro.Distro{
				BootstrapSettings: distro.BootstrapSettings{
					Method:        distro.BootstrapMethodLegacySSH,
					Communication: distro.CommunicationMethodLegacySSH,
				},
			}

			proc, err := manager.CreateProcess(ctx, &options.Create{
				Args: []string{"agent", "monitor", "command"},
			})
			require.NoError(t, err)
			proc.Tag(evergreen.AgentMonitorTag)

			mockProc, ok := proc.(*jmock.Process)
			require.True(t, ok)
			mockProc.ProcInfo.IsRunning = true

			require.NoError(t, h.StopAgentMonitor(ctx, env))

			assert.Empty(t, mockProc.Signals)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, 10*time.Second)
			defer tcancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx))
			manager := &jmock.Manager{}

			h := &Host{
				Id:   "id",
				Host: "localhost",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						Method:        distro.BootstrapMethodUserData,
						Communication: distro.CommunicationMethodRPC,
					},
				},
			}

			assert.NoError(t, withJasperServiceSetupAndTeardown(tctx, env, manager, h, func() {
				testCase(tctx, t, env, manager, h)
			}))
		})
	}
}

func TestSetupSpawnHostCommands(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, user.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, user.Collection))
	}()

	user := user.DBUser{Id: "user", APIKey: "key"}
	require.NoError(t, user.Insert())

	h := &Host{Id: "host",
		Distro: distro.Distro{
			Arch:    distro.ArchLinuxAmd64,
			WorkDir: "/dir",
			User:    "user",
			BootstrapSettings: distro.BootstrapSettings{
				JasperCredentialsPath: "/jasper_credentials_path",
			},
		},
		ProvisionOptions: &ProvisionOptions{
			OwnerId: user.Id,
		},
	}
	require.NoError(t, h.Insert())

	settings := &evergreen.Settings{
		ApiUrl: "www.example0.com",
		Ui: evergreen.UIConfig{
			Url: "www.example1.com",
		},
		HostJasper: evergreen.HostJasperConfig{
			BinaryName: "jasper_cli",
			Port:       12345,
		},
	}

	cmd, err := h.SetupSpawnHostCommands(settings)
	require.NoError(t, err)

	expected := "mkdir -m 777 -p /home/user/cli_bin" +
		" && echo '{\"api_key\":\"key\",\"api_server_host\":\"www.example0.com/api\",\"ui_server_host\":\"www.example1.com\",\"user\":\"user\"}' > /home/user/cli_bin/.evergreen.yml" +
		" && cp /home/user/evergreen /home/user/cli_bin" +
		" && (echo '\nexport PATH=\"${PATH}:/home/user/cli_bin\"\n' >> /home/user/.profile || true; echo '\nexport PATH=\"${PATH}:/home/user/cli_bin\"\n' >> /home/user/.bash_profile || true)" +
		" && chown -R user /home/user/cli_bin"
	assert.Equal(t, expected, cmd)

	h.ProvisionOptions.TaskId = "task_id"
	cmd, err = h.SetupSpawnHostCommands(settings)
	assert.Contains(t, cmd, expected)
	require.NoError(t, err)
	fetchCmd := []string{"/home/user/evergreen", "-c", "/home/user/cli_bin/.evergreen.yml", "fetch", "-t", "task_id", "--source", "--artifacts", "--dir", "/dir"}
	currIndex := 0
	for _, arg := range fetchCmd {
		foundIndex := strings.Index(cmd[currIndex:], arg)
		require.NotEqual(t, foundIndex, -1)
		currIndex += foundIndex
	}
}

func TestMarkUserDataDoneCommands(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"FailsWithoutPathToDoneFile": func(t *testing.T) {
			h := &Host{
				Id: "id",
			}
			cmd, err := h.MarkUserDataDoneCommands()
			assert.Error(t, err)
			assert.Empty(t, cmd)
		},
		"SucceedsWithPathToDoneFile": func(t *testing.T) {
			h := &Host{
				Id:     "id",
				Distro: distro.Distro{BootstrapSettings: distro.BootstrapSettings{JasperBinaryDir: "/jasper_binary_dir"}},
			}
			cmd, err := h.MarkUserDataDoneCommands()
			require.NoError(t, err)
			assert.Equal(t, "mkdir -m 777 -p /jasper_binary_dir && touch /jasper_binary_dir/user_data_done", cmd)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}

func TestSetUserDataHostProvisioned(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"Succeeds": func(t *testing.T, h *Host) {
			require.NoError(t, h.SetUserDataHostProvisioned())
			assert.Equal(t, evergreen.HostRunning, h.Status)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"IgnoresNonUserDataBootstrappedHost": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			_, err := h.Upsert()
			require.NoError(t, err)

			require.NoError(t, h.SetUserDataHostProvisioned())
			assert.Equal(t, evergreen.HostProvisioning, h.Status)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostProvisioning, dbHost.Status)
		},
		"IgnoresHostsNeverProvisioned": func(t *testing.T, h *Host) {
			h.Provisioned = false

			require.NoError(t, h.SetUserDataHostProvisioned())
			assert.Equal(t, evergreen.HostProvisioning, h.Status)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostProvisioning, dbHost.Status)
		},
		"IgnoresNonProvisioningHosts": func(t *testing.T, h *Host) {
			require.NoError(t, h.SetDecommissioned(evergreen.User, ""))

			require.NoError(t, h.SetUserDataHostProvisioned())
			assert.Equal(t, evergreen.HostDecommissioned, h.Status)

			dbHost, err := FindOneId(h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostDecommissioned, dbHost.Status)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Id: "id",
				Distro: distro.Distro{BootstrapSettings: distro.BootstrapSettings{
					Method: distro.BootstrapMethodUserData,
				}},
				Status:      evergreen.HostProvisioning,
				Provisioned: true,
			}
			require.NoError(t, h.Insert())
			testCase(t, h)
		})
	}
}

func TestCreateServicePassword(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	h := &Host{Distro: distro.Distro{
		Id: "foo",
	}}
	require.NoError(t, h.Insert())
	require.NoError(t, h.CreateServicePassword())
	assert.True(t, ValidateRDPPassword(h.ServicePassword))
	assert.NotEmpty(t, h.ServicePassword)
	dbHost, err := FindOneId(h.Id)
	require.NoError(t, err)
	assert.Equal(t, h.ServicePassword, dbHost.ServicePassword)
}

func TestSetupServiceUserCommands(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"GeneratesCommandsAndPassword": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert())
			cmds, err := h.SetupServiceUserCommands()
			require.NoError(t, err)
			assert.NotEmpty(t, cmds)
			assert.NotEmpty(t, h.ServicePassword)
		},
		"FailsWithoutServiceUser": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.ServiceUser = ""
			require.NoError(t, h.Insert())
			_, err := h.SetupServiceUserCommands()
			assert.Error(t, err)
		},
		"NoopsIfNotWindows": func(t *testing.T, h *Host) {
			h.Distro.Arch = distro.ArchLinuxAmd64
			require.NoError(t, h.Insert())
			cmds, err := h.SetupServiceUserCommands()
			assert.NoError(t, err)
			assert.Empty(t, cmds)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			testCase(t, &Host{Distro: distro.Distro{
				Arch: distro.ArchWindowsAmd64,
				BootstrapSettings: distro.BootstrapSettings{
					ServiceUser: "service-user",
				},
			}})
		})
	}
}

func newMockCredentials() (*certdepot.Credentials, error) {
	return certdepot.NewCredentials([]byte("foo"), []byte("bar"), []byte("bat"))
}

// setupCredentialsCollection sets up the credentials collection in the
// database.
func setupCredentialsCollection(ctx context.Context, env *mock.Environment) error {
	settings := env.Settings()
	settings.DomainName = "test-service"

	if err := db.ClearCollections(evergreen.CredentialsCollection, Collection); err != nil {
		return errors.WithStack(err)
	}

	maxExpiration := time.Duration(math.MaxInt64)

	bootstrapConfig := certdepot.BootstrapDepotConfig{
		CAName: evergreen.CAName,
		MongoDepot: &certdepot.MongoDBOptions{
			DatabaseName:   settings.Database.DB,
			CollectionName: evergreen.CredentialsCollection,
			DepotOptions: certdepot.DepotOptions{
				CA:                evergreen.CAName,
				DefaultExpiration: 365 * 24 * time.Hour,
			},
		},
		CAOpts: &certdepot.CertificateOptions{
			CA:         evergreen.CAName,
			CommonName: evergreen.CAName,
			Expires:    maxExpiration,
		},
		ServiceName: settings.DomainName,
		ServiceOpts: &certdepot.CertificateOptions{
			CA:         evergreen.CAName,
			CommonName: settings.DomainName,
			Host:       settings.DomainName,
			Expires:    maxExpiration,
		},
	}

	var err error
	env.Depot, err = certdepot.BootstrapDepotWithMongoClient(ctx, env.Client(), bootstrapConfig)

	return errors.WithStack(err)
}

// setupJasperService performs the necessary setup to start a local Jasper
// service associated with this host.
func setupJasperService(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *Host) (jutil.CloseFunc, error) {
	if err := h.Insert(); err != nil {
		return nil, errors.WithStack(err)
	}
	port := testutil.NextPort()
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", port))
	if err != nil {
		return nil, errors.WithStack(err)
	}
	env.Settings().HostJasper.Port = port

	creds, err := h.GenerateJasperCredentials(ctx, env)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	closeService, err := remote.StartRPCService(ctx, manager, addr, creds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return closeService, errors.WithStack(h.SaveJasperCredentials(ctx, env, creds))
}

// teardownJasperService cleans up after a Jasper service has been set up for a
// host.
func teardownJasperService(ctx context.Context, closeService jutil.CloseFunc) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(db.ClearCollections(evergreen.CredentialsCollection, Collection))
	if closeService != nil {
		catcher.Add(closeService())
	}
	return catcher.Resolve()
}

// withJasperServiceSetupAndTeardown performs necessary setup to start a
// Jasper RPC service, executes the given test function, and cleans up.
func withJasperServiceSetupAndTeardown(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *Host, fn func()) error {
	if err := setupCredentialsCollection(ctx, env); err != nil {
		grip.Error(errors.Wrap(teardownJasperService(ctx, nil), "problem tearing down test"))
		return errors.Wrapf(err, "problem setting up credentials collection")
	}

	closeService, err := setupJasperService(ctx, env, manager, h)
	if err != nil {
		grip.Error(errors.Wrap(teardownJasperService(ctx, closeService), "problem tearing down test"))
		return err
	}

	fn()

	return errors.Wrap(teardownJasperService(ctx, closeService), "problem tearing down test")
}
