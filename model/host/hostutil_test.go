package host

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/credentials"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/service/testutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/rpc"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurlCommand(t *testing.T) {
	assert := assert.New(t)
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	settings := &evergreen.Settings{
		Ui:                evergreen.UIConfig{Url: "www.example.com"},
		ClientBinariesDir: "clients",
	}
	expected := "cd ~ && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' && chmod +x evergreen.exe"
	assert.Equal(expected, h.CurlCommand(settings))

	h = &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}}
	expected = "cd ~ && curl -LO 'www.example.com/clients/linux_amd64/evergreen' && chmod +x evergreen"
	assert.Equal(expected, h.CurlCommand(settings))
}

func TestCurlCommandWithRetry(t *testing.T) {
	h := &Host{Distro: distro.Distro{Arch: distro.ArchWindowsAmd64}}
	settings := &evergreen.Settings{
		Ui:                evergreen.UIConfig{Url: "www.example.com"},
		ClientBinariesDir: "clients",
	}
	expected := "cd ~ && curl -LO 'www.example.com/clients/windows_amd64/evergreen.exe' --retry=5 --retry-max-time=10 && chmod +x evergreen.exe"
	assert.Equal(t, expected, h.CurlCommandWithRetry(settings, 5, 10))

	h = &Host{Distro: distro.Distro{Arch: distro.ArchLinuxAmd64}}
	expected = "cd ~ && curl -LO 'www.example.com/clients/linux_amd64/evergreen' --retry=5 --retry-max-time=10 && chmod +x evergreen"
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

func newMockCredentials() (*rpc.Credentials, error) {
	return rpc.NewCredentials([]byte("foo"), []byte("bar"), []byte("bat"))
}

func TestJasperCommands(t *testing.T) {
	for opName, opCase := range map[string]func(t *testing.T, h *Host, config evergreen.HostJasperConfig){
		"VerifyBaseFetchCommands": func(t *testing.T, h *Host, config evergreen.HostJasperConfig) {
			expectedCmds := []string{
				"cd \"/foo\"",
				fmt.Sprintf("curl -LO 'www.example.com/download_file-linux-amd64-abc123.tar.gz' --retry=%d --retry-max-time=%d", CurlDefaultNumRetries, CurlDefaultMaxSecs),
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
			expectedPreCmds := []string{"foo", "bar"}
			expectedPostCmds := []string{"bat", "baz"}

			creds, err := newMockCredentials()
			require.NoError(t, err)

			script, err := h.BootstrapScript(config, creds, expectedPreCmds, expectedPostCmds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "#!/bin/bash"))

			currPos := 0
			for _, expectedCmd := range expectedPreCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, offset, -1)
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset)
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedPostCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset)
				currPos += offset + len(expectedCmd)
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
				fmt.Sprintf("curl -LO 'www.example.com/download_file-windows-amd64-abc123.tar.gz' --retry=%d --retry-max-time=%d", CurlDefaultNumRetries, CurlDefaultMaxSecs),
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
			expectedPreCmds := []string{"foo", "bar"}
			expectedPostCmds := []string{"bat", "baz"}
			path := "/bin"

			for i := range expectedCmds {
				expectedCmds[i] = fmt.Sprintf("PATH=%s %s", path, expectedCmds[i])
			}

			creds, err := newMockCredentials()
			require.NoError(t, err)
			writeCredentialsCmd, err := h.writeJasperCredentialsFileCommand(config, creds)
			require.NoError(t, err)

			expectedCmds = append(expectedCmds, writeCredentialsCmd, h.ForceReinstallJasperCommand(config))

			script, err := h.BootstrapScript(config, creds, expectedPreCmds, expectedPostCmds)
			require.NoError(t, err)

			assert.True(t, strings.HasPrefix(script, "<powershell>"))
			assert.True(t, strings.HasSuffix(script, "</powershell>"))

			currPos := 0
			for _, expectedCmd := range expectedPreCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset)
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset)
				currPos += offset + len(expectedCmd)
			}

			for _, expectedCmd := range expectedPostCmds {
				offset := strings.Index(script[currPos:], expectedCmd)
				require.NotEqual(t, -1, offset)
				currPos += offset + len(expectedCmd)
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

func setupCredentialsDB(ctx context.Context, env *mock.Environment) error {
	env.Settings().DomainName = "test-service"

	if err := db.ClearCollections(credentials.Collection, Collection); err != nil {
		return errors.WithStack(err)
	}

	return errors.WithStack(credentials.Bootstrap(env))
}

func setupJasperService(ctx context.Context, env *mock.Environment, h *Host) (jasper.CloseFunc, error) {
	if _, err := h.Upsert(); err != nil {
		return nil, errors.WithStack(err)
	}
	manager := &jasper.MockManager{}
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

	closeService, err := rpc.StartService(ctx, manager, addr, creds)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return closeService, errors.WithStack(h.SaveJasperCredentials(ctx, env, creds))
}

func teardownJasperService(ctx context.Context, closeService jasper.CloseFunc) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(db.ClearCollections(credentials.Collection, Collection))
	if closeService != nil {
		catcher.Add(closeService())
	}
	return catcher.Resolve()
}

func TestJasperClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sshKeyName := "foo"
	sshKeyValue := "bar"
	for testName, testCase := range map[string]struct {
		withSetupAndTeardown func(ctx context.Context, env *mock.Environment, h *Host, fn func(ctx context.Context)) error
		h                    *Host
		expectError          bool
	}{
		"LegacyHostErrors": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					CommunicationMethod: distro.CommunicationMethodLegacySSH,
					BootstrapMethod:     distro.BootstrapMethodLegacySSH,
					SSHKey:              sshKeyName,
				},
			},
			expectError: true,
		},
		"PassesWithSSHCommunicationAndSSHInfo": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodSSH,
					SSHKey:              sshKeyName,
				},
				User: "foo",
				Host: "bar",
			},
			expectError: false,
		},
		"FailsWithSSHCommunicationButNoSSHKey": {
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodSSH,
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
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodSSH,
					SSHKey:              sshKeyName,
				},
			},
			expectError: true,
		},
		"FailsWithRPCCommunicationButNoNetworkAddress": {
			withSetupAndTeardown: func(ctx context.Context, env *mock.Environment, h *Host, fn func(ctx context.Context)) error {
				catcher := grip.NewBasicCatcher()
				if !catcher.HasErrors() {
					catcher.Add(errors.WithStack(setupCredentialsDB(ctx, env)))
				}
				var closeService jasper.CloseFunc
				var err error
				if !catcher.HasErrors() {
					closeService, err = setupJasperService(ctx, env, h)
					catcher.Add(errors.WithStack(err))
				}

				if !catcher.HasErrors() {
					fn(ctx)
				}

				catcher.Add(errors.WithStack(teardownJasperService(ctx, closeService)))
				return catcher.Resolve()
			},
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodRPC,
				},
			},
			expectError: true,
		},
		"FailsWithRPCCommunicationButNoJasperService": {
			withSetupAndTeardown: func(ctx context.Context, env *mock.Environment, h *Host, fn func(ctx context.Context)) error {
				catcher := grip.NewBasicCatcher()
				catcher.Add(errors.WithStack(setupCredentialsDB(ctx, env)))

				if !catcher.HasErrors() {
					fn(ctx)
				}

				return errors.WithStack(teardownJasperService(ctx, nil))
			},
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodRPC,
				},
				Host: "localhost",
			},
			expectError: true,
		},
		"PassesWithRPCCommunicationAndHostRunningJasperService": {
			withSetupAndTeardown: func(ctx context.Context, env *mock.Environment, h *Host, fn func(ctx context.Context)) error {
				catcher := grip.NewBasicCatcher()
				catcher.Add(errors.WithStack(setupCredentialsDB(ctx, env)))
				var closeService jasper.CloseFunc
				var err error
				if !catcher.HasErrors() {
					closeService, err = setupJasperService(ctx, env, h)
					catcher.Add(errors.WithStack(err))
				}

				if !catcher.HasErrors() {
					fn(ctx)
				}

				catcher.Add(errors.WithStack(teardownJasperService(ctx, closeService)))
				return catcher.Resolve()
			},
			h: &Host{
				Id: "test-host",
				Distro: distro.Distro{
					BootstrapMethod:     distro.BootstrapMethodSSH,
					CommunicationMethod: distro.CommunicationMethodRPC,
				},
				Host: "localhost",
			},
			expectError: false,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx, "", nil))
			env.EnvContext = tctx
			env.Settings().HostJasper.BinaryName = "binary"
			env.Settings().Keys = map[string]string{sshKeyName: sshKeyValue}

			doTest := func(ctx context.Context) {
				client, err := testCase.h.JasperClient(ctx, env)
				if testCase.expectError {
					assert.Error(t, err)
					assert.Nil(t, client)
				} else {
					assert.NoError(t, err)
					assert.NotNil(t, client)
				}
			}

			if testCase.withSetupAndTeardown != nil {
				assert.NoError(t, testCase.withSetupAndTeardown(tctx, env, testCase.h, doTest))
			} else {
				doTest(tctx)
			}
		})
	}
}

func TestJasperProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withSetupAndTeardown := func(ctx context.Context, env *mock.Environment, h *Host, fn func(context.Context)) error {
		catcher := grip.NewBasicCatcher()
		if !catcher.HasErrors() {
			catcher.Add(errors.WithStack(setupCredentialsDB(ctx, env)))
		}
		var closeService jasper.CloseFunc
		var err error
		if !catcher.HasErrors() {
			closeService, err = setupJasperService(ctx, env, h)
			catcher.Add(errors.WithStack(err))
		}

		if !catcher.HasErrors() {
			fn(ctx)
		}

		catcher.Add(errors.WithStack(teardownJasperService(ctx, closeService)))
		return catcher.Resolve()
	}
	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host){
		"RunJasperProcessErrorsWithoutJasperClient": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			assert.Error(t, h.StartJasperProcess(ctx, env, &jasper.CreateOptions{Args: []string{"echo", "hello", "world"}}))
		},
		"StartJasperProcessErrorsWithoutJasperClient": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			_, err := h.RunJasperProcess(ctx, env, "echo hello world")
			assert.Error(t, err)
		},
		"RunJasperProcessPassesWithJasperClient": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			assert.NoError(t, withSetupAndTeardown(ctx, env, h, func(ctx context.Context) {
				_, err := h.RunJasperProcess(ctx, env, "echo hello world")
				assert.NoError(t, err)
			}))
		},
		"StartJasperProcessPassesWithJasperClient": func(ctx context.Context, t *testing.T, env *mock.Environment, h *Host) {
			assert.NoError(t, withSetupAndTeardown(ctx, env, h, func(ctx context.Context) {
				assert.NoError(t, h.StartJasperProcess(ctx, env, &jasper.CreateOptions{Args: []string{"echo", "hello", "world"}}))
			}))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env := &mock.Environment{}
			require.NoError(t, env.Configure(tctx, "", nil))
			env.EnvContext = tctx
			env.Settings().HostJasper.BinaryName = "binary"

			testCase(tctx, t, env, &Host{
				Id: "test",
				Distro: distro.Distro{
					CommunicationMethod: distro.CommunicationMethodRPC,
					BootstrapMethod:     distro.BootstrapMethodUserData,
				},
				Host: "localhost",
			})
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

func TestBuildLocalJasperClientRequest(t *testing.T) {
	h := &Host{
		Distro: distro.Distro{
			CuratorDir:            "/curator",
			JasperCredentialsPath: "/jasper/credentials",
		},
	}

	config := evergreen.HostJasperConfig{Port: 12345, BinaryName: "binary"}
	input := struct {
		Field string `json:"field"`
	}{Field: "foo"}
	subCmd := "sub"

	config.BinaryName = "binary"
	binaryName := h.jasperBinaryFilePath(config)

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
	assert.Contains(t, cmd[index:], "--port=12345")
	assert.Contains(t, cmd[index:], "--creds_path=/jasper/credentials")
}

func TestSetupScriptCommands(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, h *Host, settings *evergreen.Settings){
		"ReturnsEmptyWithoutSetup": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			cmds, err := h.SetupScriptCommands(settings)
			require.NoError(t, err)
			assert.Empty(t, cmds)
		},
		"ReturnsEmptyIfSpawnedByTask": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.SpawnOptions.SpawnedByTask = true
			cmds, err := h.SetupScriptCommands(settings)
			require.NoError(t, err)
			assert.Empty(t, cmds)
		},
		"ReturnsUnmodifiedWithoutExpansions": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			h.Distro.Setup = "foo"
			cmds, err := h.SetupScriptCommands(settings)
			require.NoError(t, err)
			assert.Equal(t, h.Distro.Setup, cmds)
		},
		"ReturnsExpandedWithValidExpansions": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			key := "foo"
			h.Distro.Setup = fmt.Sprintf("${%s}", key)
			settings.Expansions = map[string]string{
				key: "bar",
			}
			cmds, err := h.SetupScriptCommands(settings)
			require.NoError(t, err)
			assert.Equal(t, settings.Expansions[key], cmds)
		},
		"FailsWithInvalidSetup": func(t *testing.T, h *Host, settings *evergreen.Settings) {
			key := "foo"
			h.Distro.Setup = "${" + key
			settings.Expansions = map[string]string{
				key: "bar",
			}
			_, err := h.SetupScriptCommands(settings)
			assert.Error(t, err)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, &Host{Id: "test"}, &evergreen.Settings{})
		})
	}
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

	expectedCmd, err := json.Marshal(h.agentMonitorCommand(settings))
	require.NoError(t, err)
	assert.Contains(t, cmd, string(expectedCmd))

	expectedEnv, err := json.Marshal(buildAgentEnv(settings))
	require.NoError(t, err)
	assert.Contains(t, cmd, string(expectedEnv))
}

func TestSetupSpawnHostCommand(t *testing.T) {
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
	}

	cmd, err := h.SetupSpawnHostCommand(settings)
	require.NoError(t, err)

	expected := `mkdir -m 777 -p ~/cli_bin && echo '{"api_key":"key","api_server_host":"www.example0.com/api","ui_server_host":"www.example1.com","user":"user"}' > ~/cli_bin/.evergreen.yml && cp ~/evergreen ~/cli_bin && (echo 'PATH=${PATH}:~/cli_bin' >> ~/.profile || true; echo 'PATH=${PATH}:~/cli_bin' >> ~/.bash_profile || true)`
	assert.Equal(t, expected, cmd)

	h.ProvisionOptions.TaskId = "task_id"
	cmd, err = h.SetupSpawnHostCommand(settings)
	require.NoError(t, err)
	expected += " && ~/evergreen -c ~/cli_bin/.evergreen.yml fetch -t task_id --source --artifacts --dir='/dir'"
	assert.Equal(t, expected, cmd)
}
