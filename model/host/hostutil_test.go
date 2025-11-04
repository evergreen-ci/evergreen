package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/userdata"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
	jmock "github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	jutil "github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommand(t *testing.T) {
	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			Api: evergreen.APIConfig{URL: "www.example.com"},
		},
		Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"},
	}

	t.Run("Linux", func(t *testing.T) {
		h := &Host{
			Distro: distro.Distro{
				Arch: evergreen.ArchLinuxAmd64,
				User: "user",
			},
			User: "user",
		}
		expected := "cd /home/user && curl -fLO https://foo.com/linux_amd64/evergreen && chmod +x evergreen"
		cmd, err := h.CurlCommand(env)
		require.NoError(t, err)
		assert.Equal(t, expected, cmd)
	})
	t.Run("MacOS", func(t *testing.T) {
		h := &Host{
			Distro: distro.Distro{
				Arch: evergreen.ArchDarwinArm64,
				User: "user",
			},
			User: "user",
		}
		expected := "cd /Users/user && rm -f evergreen && curl -fLO https://foo.com/darwin_arm64/evergreen && chmod +x evergreen"
		cmd, err := h.CurlCommand(env)
		require.NoError(t, err)
		assert.Equal(t, expected, cmd)
	})
}

func TestSpawnHostGetTaskDataCommand(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h := &Host{
		Id: "host_id",
		ProvisionOptions: &ProvisionOptions{
			TaskId: "task_id",
		},
		Distro: distro.Distro{
			WorkDir: "/some/directory",
		},
	}
	expected := []string{"/home/evergreen", "-c", "/home/.evergreen.yml", "fetch", "-t", "task_id", "--source", "--artifacts", "--dir", "/some/directory", "--use-app-token", "--revoke-tokens", "--token", "gh_something_token", "-m", "module:gh_module_token", "-m", "module2:gh_module2_token"}
	cmd := h.SpawnHostGetTaskDataCommand(ctx, "gh_something_token", []string{"module:gh_module_token", "module2:gh_module2_token"})
	assert.Equal(t, expected, cmd)
}

func TestCurlCommandWithRetry(t *testing.T) {
	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			Api: evergreen.APIConfig{URL: "www.example.com"},
		},
		Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"},
	}
	h := &Host{
		Distro: distro.Distro{
			Arch: evergreen.ArchLinuxAmd64,
			User: "user",
		},
		User: "user",
	}
	expected := "cd /home/user && curl -fLO https://foo.com/linux_amd64/evergreen --retry 5 --retry-max-time 10 && chmod +x evergreen"
	cmd, err := h.CurlCommandWithRetry(env, 5, 10)
	require.NoError(t, err)
	assert.Equal(t, expected, cmd)
}

func TestGetSSHOptions(t *testing.T) {
	keyFile, err := os.CreateTemp(t.TempDir(), "")
	require.NoError(t, err)
	defaultKeyPath := keyFile.Name()

	checkContainsOptionsAndValues := func(t *testing.T, expected []string, actual []string) {
		exists := map[string]bool{}
		require.Equal(t, len(expected)%2, 0, `expected options must be in pairs (e.g. ("-o", "LogLevel=DEBUG"))`)
		require.Equal(t, len(actual)%2, 0, `actual options must be in pairs (e.g. ("-o", "LogLevel=DEBUG"))`)
		for i := 0; i < len(actual); i += 2 {
			exists[actual[i]+actual[i+1]] = true
		}
		for i := 0; i < len(expected); i += 2 {
			assert.True(t, exists[expected[i]+expected[i+1]], "missing (\"%s\",\"%s\")", expected[i], expected[i+1])
		}
	}
	for testName, testCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"ReturnsExpectedArguments": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expected := []string{"-i", defaultKeyPath, "-o", "UserKnownHostsFile=/dev/null", "-o", "RequestTTY=no"}
			opts, err := h.GetSSHOptions(settings)
			require.NoError(t, err)
			checkContainsOptionsAndValues(t, expected, opts)
		},
		"SetsDistroPortIfHostSpecificPortIsUnspecified": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.SSHOptions = append(h.Distro.SSHOptions, "Port=123")
			expected := []string{"-i", defaultKeyPath, "-o", "UserKnownHostsFile=/dev/null", "-o", "Port=123", "-o", "RequestTTY=no"}
			opts, err := h.GetSSHOptions(settings)
			require.NoError(t, err)
			checkContainsOptionsAndValues(t, expected, opts)
		},
		"PrioritizesHostSpecificPortOverDistroPort": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.SSHOptions = append(h.Distro.SSHOptions, "Port=456")
			h.SSHPort = 123
			expected := []string{"-i", defaultKeyPath, "-o", "UserKnownHostsFile=/dev/null", "-o", "Port=123", "-o", "RequestTTY=no"}
			opts, err := h.GetSSHOptions(settings)
			require.NoError(t, err)
			checkContainsOptionsAndValues(t, expected, opts)
		},
		"FailsWithoutIdentityFile": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			settings.KanopySSHKeyPath = "does_not_exist"

			_, err := h.GetSSHOptions(settings)
			assert.Error(t, err)
		},
		"IncludesAdditionalArguments": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.SSHOptions = []string{"UserKnownHostsFile=/path/to/file"}

			expected := []string{"-i", defaultKeyPath, "-o", h.Distro.SSHOptions[0]}
			opts, err := h.GetSSHOptions(settings)
			require.NoError(t, err)
			checkContainsOptionsAndValues(t, expected, opts)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, &Host{
				Id: "id",
			}, &evergreen.Settings{
				KanopySSHKeyPath: defaultKeyPath,
			})
		})
	}
}

func TestJasperCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := []string{
				"cd /foo",
				fmt.Sprintf("curl -fLO www.example.com/download_file-linux-amd64-abc123.tar.gz --retry %d --retry-max-time %d", curlDefaultNumRetries, curlDefaultMaxSecs),
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
		"GenerateUserDataProvisioningScriptForAgent": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.StartedBy = evergreen.User
			require.NoError(t, h.Insert(ctx))

			checkRerun := h.CheckUserDataProvisioningStartedCommand()

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			startAgentMonitor, err := h.StartAgentMonitorRequest(ctx, settings)
			require.NoError(t, err)

			markDone := h.MarkUserDataProvisioningDoneCommand()

			expectedCmds := []string{
				checkRerun,
				setupScript,
				h.MakeJasperDirsCommand(),
				h.FetchJasperCommand(settings.HostJasper),

				h.ForceReinstallJasperCommand(settings),
				h.ChangeJasperDirsOwnerCommand(),
				startAgentMonitor,
				markDone,
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.GenerateUserDataProvisioningScript(ctx, settings, creds, "", []string{})
			require.NoError(t, err)

			assertStringContainsOrderedSubstrings(t, script, expectedCmds)
		},
		"GenerateUserDataProvisioningScriptForSpawnHostUsingStaticCredentials": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			h.StartedBy = "started_by_user"
			h.UserHost = true
			userID := "user"
			user := &user.DBUser{Id: userID}
			require.NoError(t, user.Insert(t.Context()))

			h.ProvisionOptions = &ProvisionOptions{
				OwnerId: userID,
				TaskId:  "task_id",
			}
			require.NoError(t, h.Insert(ctx))

			checkRerun := h.CheckUserDataProvisioningStartedCommand()

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			setupSpawnHost, err := h.SpawnHostSetupCommands(t.Context(), settings)
			require.NoError(t, err)

			markDone := h.MarkUserDataProvisioningDoneCommand()

			expectedCmds := []string{
				checkRerun,
				setupScript,
				h.MakeJasperDirsCommand(),
				h.FetchJasperCommand(settings.HostJasper),

				h.ForceReinstallJasperCommand(settings),
				h.ChangeJasperDirsOwnerCommand(),
				setupSpawnHost,
				markDone,
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.GenerateUserDataProvisioningScript(ctx, settings, creds, "", []string{})
			require.NoError(t, err)

			assertStringContainsOrderedSubstrings(t, script, expectedCmds)
		},
		"GenerateUserDataProvisioningScriptForSpawnHostUsingOAuth": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			settings.AuthConfig.OAuth = &evergreen.OAuthConfig{
				Issuer:      "issuer_url_with'_some'_quotes",
				ClientID:    "client_id",
				ConnectorID: "connector_id",
			}
			h.StartedBy = "started_by_user"
			h.UserHost = true
			userID := "user"
			user := &user.DBUser{Id: userID}
			require.NoError(t, user.Insert(t.Context()))

			h.ProvisionOptions = &ProvisionOptions{
				OwnerId:  userID,
				TaskId:   "task_id",
				UseOAuth: true,
			}
			require.NoError(t, h.Insert(ctx))

			checkRerun := h.CheckUserDataProvisioningStartedCommand()

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			setupSpawnHost, err := h.SpawnHostSetupCommands(t.Context(), settings)
			require.NoError(t, err)

			markDone := h.MarkUserDataProvisioningDoneCommand()

			expectedCmds := []string{
				checkRerun,
				setupScript,
				h.MakeJasperDirsCommand(),
				h.FetchJasperCommand(settings.HostJasper),

				h.ForceReinstallJasperCommand(settings),
				h.ChangeJasperDirsOwnerCommand(),
				setupSpawnHost,
				markDone,
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.GenerateUserDataProvisioningScript(ctx, settings, creds, "", []string{})
			require.NoError(t, err)
			assert.Contains(t, script, "issuer: issuer_url_with'_some'_quotes")
			assert.Contains(t, script, "do_not_use_browser: true")

			assertStringContainsOrderedSubstrings(t, script, expectedCmds)
		},
		"ForceReinstallJasperCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.User))
		},
		"ForceReinstallJasperCommandWithEnvVars": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.Env = []distro.EnvVar{
				{Key: "envKey0", Value: "envValue0"},
				{Key: "envKey1", Value: "envValue1"},
			}
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.User))
			assert.Contains(t, cmd, "--env 'envKey0=envValue0'")
			assert.Contains(t, cmd, "--env 'envKey1=envValue1'")
		},
		"ForceReinstallJasperWithResourceLimits": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.ResourceLimits = distro.ResourceLimits{
				NumProcesses:    1,
				NumFiles:        2,
				NumTasks:        3,
				LockedMemoryKB:  4,
				VirtualMemoryKB: 5,
			}
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.User))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_num_procs=%d", h.Distro.BootstrapSettings.ResourceLimits.NumProcesses))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_num_files=%d", h.Distro.BootstrapSettings.ResourceLimits.NumFiles))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_num_tasks=%d", h.Distro.BootstrapSettings.ResourceLimits.NumTasks))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_virtual_memory=%d", h.Distro.BootstrapSettings.ResourceLimits.VirtualMemoryKB))
			assert.Contains(t, cmd, fmt.Sprintf("--limit_locked_memory=%d", h.Distro.BootstrapSettings.ResourceLimits.LockedMemoryKB))
		},
		"ForceReinstallJasperCommandWithPrecondition": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.PreconditionScripts = []distro.PreconditionScript{
				{Path: "/tmp/precondition1.sh", Script: "#!/bin/bash\necho hello"},
				{Path: "/tmp/precondition2.sh", Script: "#!/bin/bash\necho world"},
			}
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, "sudo /foo/jasper_cli jasper service force-reinstall rpc"))

			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf("--user=%s", h.User))
			for _, ps := range h.Distro.BootstrapSettings.PreconditionScripts {
				assert.Contains(t, cmd, fmt.Sprintf("--precondition=%s", ps.Path))
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, evergreen.CredentialsCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, evergreen.CredentialsCollection))
			}()
			h := &Host{
				Distro: distro.Distro{
					Arch: evergreen.ArchLinuxAmd64,
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						Communication:         distro.CommunicationMethodRPC,
						ShellPath:             "/bin/bash",
						JasperBinaryDir:       "/foo",
						JasperCredentialsPath: "/bar/bat.txt",
					},
					Setup: "#!/bin/bash\necho hello world",
				},
				StartedBy: evergreen.User,
				User:      "user",
			}
			settings := &evergreen.Settings{
				HostInit: evergreen.HostInitConfig{},
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			expectedCmds := []string{
				"cd /foo",
				fmt.Sprintf("curl -fLO www.example.com/download_file-windows-amd64-abc123.tar.gz --retry %d --retry-max-time %d", curlDefaultNumRetries, curlDefaultMaxSecs),
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
		"UserDataProvisioningScriptForAgent": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, h.Insert(ctx))

			checkRerun := h.CheckUserDataProvisioningStartedCommand()

			setupUser, err := h.SetupServiceUserCommands(ctx)
			require.NoError(t, err)

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			creds, err := newMockCredentials()
			require.NoError(t, err)
			writeCredentialsCmd, err := h.WriteJasperCredentialsFilesCommands(creds)
			require.NoError(t, err)

			startAgentMonitor, err := h.StartAgentMonitorRequest(ctx, settings)
			require.NoError(t, err)

			markDone := h.MarkUserDataProvisioningDoneCommand()

			var expectedCmds []string
			expectedCmds = append(expectedCmds, checkRerun, setupUser, setupScript, writeCredentialsCmd)
			expectedCmds = append(expectedCmds,
				h.FetchJasperCommand(settings.HostJasper),

				h.ForceReinstallJasperCommand(settings),
				h.ChangeJasperDirsOwnerCommand(),
				startAgentMonitor,
				markDone,
			)

			script, err := h.GenerateUserDataProvisioningScript(ctx, settings, creds, "", []string{})
			require.NoError(t, err)

			assertStringContainsOrderedSubstrings(t, script, expectedCmds)
		},
		"ProvisioningUserDataForSpawnHost": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			require.NoError(t, db.Clear(user.Collection))
			defer func() {
				assert.NoError(t, db.Clear(user.Collection))
			}()
			h.StartedBy = "started_by_user"
			h.UserHost = true
			userID := "user"
			user := &user.DBUser{Id: userID}
			require.NoError(t, user.Insert(t.Context()))
			h.ProvisionOptions = &ProvisionOptions{
				OwnerId: userID,
			}
			require.NoError(t, h.Insert(ctx))

			checkRerun := h.CheckUserDataProvisioningStartedCommand()

			setupUser, err := h.SetupServiceUserCommands(ctx)
			require.NoError(t, err)

			setupScript, err := h.setupScriptCommands(settings)
			require.NoError(t, err)

			creds, err := newMockCredentials()
			require.NoError(t, err)
			writeCredentialsCmd, err := h.WriteJasperCredentialsFilesCommands(creds)
			require.NoError(t, err)

			setupSpawnHost, err := h.SpawnHostSetupCommands(t.Context(), settings)
			require.NoError(t, err)

			markDone := h.MarkUserDataProvisioningDoneCommand()

			var expectedCmds []string
			expectedCmds = append(expectedCmds, checkRerun, setupUser, setupScript, writeCredentialsCmd)
			expectedCmds = append(expectedCmds,
				h.FetchJasperCommand(settings.HostJasper),

				h.ForceReinstallJasperCommand(settings),
				h.ChangeJasperDirsOwnerCommand(),
				setupSpawnHost,
				markDone,
			)

			script, err := h.GenerateUserDataProvisioningScript(ctx, settings, creds, "", []string{})
			require.NoError(t, err)

			assertStringContainsOrderedSubstrings(t, script, expectedCmds)
		},
		"ForceReinstallJasperCommandSSH": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			require.NoError(t, h.Insert(ctx))
			require.NoError(t, h.CreateServicePassword(ctx))
			cmd := h.ForceReinstallJasperCommand(settings)
			assert.True(t, strings.HasPrefix(cmd, fmt.Sprintf("JASPER_USER_PASSWORD=%s /foo/jasper_cli.exe jasper service force-reinstall rpc", h.ServicePassword)))
			assert.Contains(t, cmd, "--host=0.0.0.0")
			assert.Contains(t, cmd, fmt.Sprintf("--port=%d", settings.HostJasper.Port))
			assert.Contains(t, cmd, fmt.Sprintf("--creds_path=%s", h.Distro.BootstrapSettings.JasperCredentialsPath))
			assert.Contains(t, cmd, fmt.Sprintf(`--user=.\\%s`, h.Distro.BootstrapSettings.ServiceUser))
		},
		"WriteJasperCredentialsFileCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			creds, err := newMockCredentials()
			require.NoError(t, err)

			cmd, err := h.WriteJasperCredentialsFilesCommands(creds)
			require.NoError(t, err)

			expectedCreds, err := creds.Export()
			require.NoError(t, err)
			assert.Equal(t, fmt.Sprintf("echo '%s' > /bar/bat.txt && chmod 666 /bar/bat.txt", expectedCreds), cmd)
		},
		"WriteJasperPreconditionScriptsCommand": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.BootstrapSettings.PreconditionScripts = []distro.PreconditionScript{
				{Path: "/tmp/precondition1.sh", Script: "#!/bin/bash\necho hello"},
				{Path: "/tmp/precondition2.sh", Script: "#!/bin/bash\necho world"},
			}

			cmd := h.WriteJasperPreconditionScriptsCommands()
			assert.Contains(t, cmd, "tee /tmp/precondition1.sh <<'EOF'\n#!/bin/bash\necho hello\nEOF\nchmod 755 /tmp/precondition1.sh")
			assert.Contains(t, cmd, "tee /tmp/precondition2.sh <<'EOF'\n#!/bin/bash\necho world\nEOF\nchmod 755 /tmp/precondition2.sh")
		},
	} {
		t.Run(opName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(Collection, evergreen.CredentialsCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(Collection, evergreen.CredentialsCollection))
			}()
			h := &Host{
				Distro: distro.Distro{
					Arch: evergreen.ArchWindowsAmd64,
					BootstrapSettings: distro.BootstrapSettings{
						Method:                distro.BootstrapMethodUserData,
						Communication:         distro.CommunicationMethodRPC,
						JasperBinaryDir:       "/foo",
						JasperCredentialsPath: "/bar/bat.txt",
						ShellPath:             "/bin/bash",
						ServiceUser:           "service-user",
					},
					Setup: "#!/bin/bash\necho hello",
				},
				StartedBy: evergreen.User,
				User:      "user",
			}
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
				},
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

			keyFile, err := os.CreateTemp(t.TempDir(), "")
			require.NoError(t, err)
			env.Settings().KanopySSHKeyPath = keyFile.Name()

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.ClearCollections(Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection))
	}()

	h := &Host{Id: "id", Distro: distro.Distro{WorkDir: "/foo"}}
	require.NoError(t, h.Insert(ctx))

	settings := &evergreen.Settings{
		Api: evergreen.APIConfig{URL: "www.example0.com"},
		LoggerConfig: evergreen.LoggerConfig{
			LogkeeperURL: "www.example1.com",
		},
		Ui: evergreen.UIConfig{
			Url: "www.example2.com",
		},
	}

	cmd, err := h.StartAgentMonitorRequest(ctx, settings)
	require.NoError(t, err)

	assert.NotEmpty(t, h.Secret)
	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.Equal(t, h.Secret, dbHost.Secret)

	expectedCmd, err := json.Marshal(h.AgentMonitorOptions(settings))
	require.NoError(t, err)
	assert.Contains(t, cmd, string(expectedCmd))

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

func TestSpawnHostSetupCommands(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, user.Collection))
	defer func() {
		assert.NoError(t, db.ClearCollections(Collection, user.Collection))
	}()

	user := user.DBUser{Id: "user", APIKey: "key"}
	require.NoError(t, user.Insert(t.Context()))

	h := &Host{Id: "host",
		Distro: distro.Distro{
			Arch:    evergreen.ArchLinuxAmd64,
			WorkDir: "/dir",
			BootstrapSettings: distro.BootstrapSettings{
				Method:                distro.BootstrapMethodUserData,
				Communication:         distro.CommunicationMethodRPC,
				JasperCredentialsPath: "/jasper_credentials_path",
			},
			User: user.Id,
		},
		ProvisionOptions: &ProvisionOptions{
			OwnerId: user.Id,
		},
		User: user.Id,
	}
	require.NoError(t, h.Insert(t.Context()))

	getExpected := func(oauth bool) string {
		expected := "mkdir -m 777 -p /home/user/cli_bin" +
			" && (sudo chown -R user /home/user/.evergreen.yml || true)" +
			" && echo \"user: user\napi_key: key\napi_server_host: www.example0.com/api\nui_server_host: www.example1.com\n"
		if oauth {
			expected += "oauth:\n    issuer: https://www.example.com\n    client_id: client_id\n    connector_id: connector_id\n    do_not_use_browser: true\n"
		}
		expected += "\" > /home/user/.evergreen.yml" +
			" && chmod +x /home/user/evergreen" +
			" && cp /home/user/evergreen /home/user/cli_bin" +
			" && (echo '\nexport PATH=\"${PATH}:/home/user/cli_bin\"\n' >> /home/user/.profile || true; echo '\nexport PATH=\"${PATH}:/home/user/cli_bin\"\n' >> /home/user/.bash_profile || true)" +
			" && (sudo chown -R user /home/user/.profile /home/user/.bash_profile || true)"
		return expected
	}

	getSettings := func() *evergreen.Settings {
		return &evergreen.Settings{
			Api: evergreen.APIConfig{URL: "www.example0.com"},
			Ui: evergreen.UIConfig{
				Url: "www.example1.com",
			},
			HostJasper: evergreen.HostJasperConfig{
				BinaryName: "jasper_cli",
				Port:       12345,
			},
		}
	}

	t.Run("WithoutOAuth", func(t *testing.T) {
		cmd, err := h.SpawnHostSetupCommands(t.Context(), getSettings())
		require.NoError(t, err)
		assert.Equal(t, getExpected(false), cmd)
	})

	t.Run("WithOAuth", func(t *testing.T) {
		settings := getSettings()
		settings.AuthConfig = evergreen.AuthConfig{
			OAuth: &evergreen.OAuthConfig{
				Issuer:      "https://www.example.com",
				ClientID:    "client_id",
				ConnectorID: "connector_id",
			},
		}

		cmd, err := h.SpawnHostSetupCommands(t.Context(), settings)
		require.NoError(t, err)
		assert.Equal(t, getExpected(true), cmd)
	})
}

func TestAddPublicKeyScript(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, h *Host){
		"CreatesExpectedScript": func(t *testing.T, h *Host) {
			expected := "grep -qxF \"public_key\" /home/user/.ssh/authorized_keys || echo \"\npublic_key\" >> /home/user/.ssh/authorized_keys"
			actual := h.AddPublicKeyScript("public_key")
			assert.Equal(t, expected, actual)
		},
		"TrimsPublicKeysWithNewlinesToSingleLinePublicKey": func(t *testing.T, h *Host) {
			expected := "grep -qxF \"public_key\" /home/user/.ssh/authorized_keys || echo \"\npublic_key\" >> /home/user/.ssh/authorized_keys"
			actual := h.AddPublicKeyScript("\npublic_key\n")
			assert.Equal(t, expected, actual)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					User: "user",
				},
			}
			tCase(t, h)
		})
	}
}

func TestCheckUserDataProvisioningStartedCommand(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"CreatesExpectedCommand": func(t *testing.T, h *Host) {
			expectedCmd := "[ -a /jasper_binary_dir/user_data_started ] && exit" +
				" || mkdir -m 777 -p /jasper_binary_dir && touch /jasper_binary_dir/user_data_started"
			cmd := h.CheckUserDataProvisioningStartedCommand()
			assert.Equal(t, expectedCmd, cmd)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			h := &Host{
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						JasperBinaryDir: "/jasper_binary_dir",
					},
				},
			}
			testCase(t, h)
		})
	}
}

func TestMarkUserDataProvisioningDoneCommand(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T){
		"CreatesExpectedCommand": func(t *testing.T) {
			h := &Host{
				Id: "id",
				Distro: distro.Distro{
					BootstrapSettings: distro.BootstrapSettings{
						JasperBinaryDir: "/jasper_binary_dir",
					},
				},
			}
			cmd := h.MarkUserDataProvisioningDoneCommand()
			assert.Equal(t, "touch /jasper_binary_dir/user_data_done", cmd)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t)
		})
	}
}

func TestSetUserDataHostProvisioned(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"Succeeds": func(t *testing.T, h *Host) {
			require.NoError(t, h.SetUserDataHostProvisioned(ctx))
			assert.Equal(t, evergreen.HostRunning, h.Status)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostRunning, dbHost.Status)
		},
		"IgnoresNonUserDataBootstrappedHost": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.Method = distro.BootstrapMethodSSH
			_, err := h.Upsert(ctx)
			require.NoError(t, err)

			require.NoError(t, h.SetUserDataHostProvisioned(ctx))
			assert.Equal(t, evergreen.HostStarting, h.Status)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStarting, dbHost.Status)
		},
		"IgnoresHostsNeverProvisioned": func(t *testing.T, h *Host) {
			h.Provisioned = false

			require.NoError(t, h.SetUserDataHostProvisioned(ctx))
			assert.Equal(t, evergreen.HostStarting, h.Status)

			dbHost, err := FindOneId(ctx, h.Id)
			require.NoError(t, err)
			assert.Equal(t, evergreen.HostStarting, dbHost.Status)
		},
		"IgnoresNonStartingHosts": func(t *testing.T, h *Host) {
			require.NoError(t, h.SetDecommissioned(ctx, evergreen.User, true, ""))

			require.NoError(t, h.SetUserDataHostProvisioned(ctx))
			assert.Equal(t, evergreen.HostDecommissioned, h.Status)

			dbHost, err := FindOneId(ctx, h.Id)
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
				Status:      evergreen.HostStarting,
				Provisioned: true,
			}
			require.NoError(t, h.Insert(ctx))
			testCase(t, h)
		})
	}
}

func TestCreateServicePassword(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, db.Clear(Collection))
	defer func() {
		assert.NoError(t, db.Clear(Collection))
	}()
	h := &Host{Distro: distro.Distro{
		Id: "foo",
	}}
	require.NoError(t, h.Insert(ctx))
	require.NoError(t, h.CreateServicePassword(ctx))
	assert.True(t, ValidateRDPPassword(h.ServicePassword))
	assert.NotEmpty(t, h.ServicePassword)
	dbHost, err := FindOneId(ctx, h.Id)
	require.NoError(t, err)
	assert.Equal(t, h.ServicePassword, dbHost.ServicePassword)
}

func TestSetupServiceUserCommands(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"GeneratesCommandsAndPassword": func(t *testing.T, h *Host) {
			require.NoError(t, h.Insert(ctx))
			cmds, err := h.SetupServiceUserCommands(ctx)
			require.NoError(t, err)
			assert.NotEmpty(t, cmds)
			assert.NotEmpty(t, h.ServicePassword)
		},
		"FailsWithoutServiceUser": func(t *testing.T, h *Host) {
			h.Distro.BootstrapSettings.ServiceUser = ""
			require.NoError(t, h.Insert(ctx))
			_, err := h.SetupServiceUserCommands(ctx)
			assert.Error(t, err)
		},
		"NoopsIfNotWindows": func(t *testing.T, h *Host) {
			h.Distro.Arch = evergreen.ArchLinuxAmd64
			require.NoError(t, h.Insert(ctx))
			cmds, err := h.SetupServiceUserCommands(ctx)
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
				Arch: evergreen.ArchWindowsAmd64,
				BootstrapSettings: distro.BootstrapSettings{
					ServiceUser: "service-user",
				},
			}})
		})
	}
}

func TestMakeJasperDirsCommand(t *testing.T) {
	h := &Host{
		Distro: distro.Distro{
			BootstrapSettings: distro.BootstrapSettings{
				JasperBinaryDir:       "/jasper_binary_dir",
				JasperCredentialsPath: "/jasper_credentials_path/file",
				ClientDir:             "/jasper_client_dir",
			},
		},
	}
	assert.Equal(t, "mkdir -m 777 -p /jasper_binary_dir /jasper_credentials_path /jasper_client_dir", h.MakeJasperDirsCommand())
}

func TestChangeJasperDirsOwnerCommand(t *testing.T) {
	t.Run("NonWindowsHost", func(t *testing.T) {
		h := &Host{
			Distro: distro.Distro{
				Arch: evergreen.ArchLinuxAmd64,
				BootstrapSettings: distro.BootstrapSettings{
					JasperBinaryDir:       "/jasper_binary_dir",
					JasperCredentialsPath: "/jasper_credentials_path/file",
					ClientDir:             "/jasper_client_dir",
				},
			},
			User: "user",
		}
		assert.Equal(t, "sudo chown -R user /jasper_binary_dir && sudo chown -R user /jasper_credentials_path && sudo chown -R user /jasper_client_dir", h.ChangeJasperDirsOwnerCommand())
	})

	t.Run("WindowsHost", func(t *testing.T) {
		h := &Host{
			Distro: distro.Distro{
				Arch: evergreen.ArchWindowsAmd64,
				BootstrapSettings: distro.BootstrapSettings{
					JasperBinaryDir:       "/jasper_binary_dir",
					JasperCredentialsPath: "/jasper_credentials_path/file",
					ClientDir:             "/jasper_client_dir",
				},
			},
			User: "user",
		}
		assert.Equal(t, "chown -R user /jasper_binary_dir && chown -R user /jasper_credentials_path && chown -R user /jasper_client_dir", h.ChangeJasperDirsOwnerCommand())
	})
}

func TestGenerateFetchProvisioningScriptUserData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{
		EvergreenSettings: &evergreen.Settings{
			Api: evergreen.APIConfig{URL: "https://example.com"},
		},
		Clients: evergreen.ClientConfig{S3URLPrefix: "https://foo.com"},
	}
	for testName, testCase := range map[string]func(t *testing.T, h *Host){
		"Linux": func(t *testing.T, h *Host) {
			h.Distro.Arch = evergreen.ArchLinuxAmd64
			opts, err := h.GenerateFetchProvisioningScriptUserData(ctx, env)
			require.NoError(t, err)

			makeJasperDirs := h.MakeJasperDirsCommand()
			fetchClient, err := h.CurlCommandWithDefaultRetry(env)
			fixClientOwner := h.changeOwnerCommand(filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()))
			require.NoError(t, err)

			expectedParts := []string{
				makeJasperDirs,
				fetchClient,
				fixClientOwner,
				"/home/user/evergreen host provision",
				"--api_server=https://example.com",
				"--host_id=host_id",
				"--host_secret=host_secret",
				"--working_dir=/jasper_binary_dir",
				"--shell_path=/bin/bash",
			}

			assertStringContainsOrderedSubstrings(t, opts.Content, expectedParts)
			assert.Equal(t, userdata.ShellScript+userdata.Directive(h.Distro.BootstrapSettings.ShellPath), opts.Directive)
		},
		"Windows": func(t *testing.T, h *Host) {
			h.Distro.Arch = evergreen.ArchWindowsAmd64

			opts, err := h.GenerateFetchProvisioningScriptUserData(ctx, env)
			require.NoError(t, err)

			makeJasperDirs := h.MakeJasperDirsCommand()
			fetchClient, err := h.CurlCommandWithDefaultRetry(env)
			require.NoError(t, err)
			fixClientOwner := h.changeOwnerCommand(filepath.Join(h.Distro.HomeDir(), h.Distro.BinaryName()))

			expectedParts := []string{
				makeJasperDirs,
				fetchClient,
				fixClientOwner,
				"/home/user/evergreen.exe host provision",
				"--api_server=https://example.com",
				"--host_id=host_id",
				"--host_secret=host_secret",
				"--working_dir=/jasper_binary_dir",
				"--shell_path=/bin/bash",
			}
			for _, part := range expectedParts {
				assert.Contains(t, opts.Content, part)
			}
			assert.Equal(t, userdata.PowerShellScript, opts.Directive)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			h := &Host{
				Id: "host_id",
				Distro: distro.Distro{
					Id: "distro_id",
					BootstrapSettings: distro.BootstrapSettings{
						JasperBinaryDir: "/jasper_binary_dir",
						ShellPath:       "/bin/bash",
					},
					User: "user",
				},
				Secret: "host_secret",
				User:   "user",
			}
			require.NoError(t, h.Insert(ctx))

			testCase(t, h)
		})
	}
}

func assertStringContainsOrderedSubstrings(t *testing.T, s string, subs []string) {
	var currPos int
	for _, sub := range subs {
		require.Less(t, currPos, len(s), "substring '%s' not found", sub)
		offset := strings.Index(s[currPos:], sub)
		require.NotEqual(t, "missing '%s'", -1, sub)
		currPos += offset + len(sub)
	}
}

func newMockCredentials() (*certdepot.Credentials, error) {
	return certdepot.NewCredentials([]byte("foo"), []byte("bar"), []byte("bat"))
}

// setupJasperService performs the necessary setup to start a local Jasper
// service associated with this host.
func setupJasperService(ctx context.Context, env *mock.Environment, manager *jmock.Manager, h *Host) (jutil.CloseFunc, error) {
	if err := h.Insert(ctx); err != nil {
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
	closeService, err := setupJasperService(ctx, env, manager, h)
	if err != nil {
		grip.Error(errors.Wrap(teardownJasperService(ctx, closeService), "problem tearing down test"))
		return err
	}

	fn()

	return errors.Wrap(teardownJasperService(ctx, closeService), "problem tearing down test")
}
