package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSSHClient(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, remoteOpts options.Remote, clientOpts ClientOptions){
		"NewSSHClientFailsWithEmptyRemoteOptions": func(t *testing.T, remoteOpts options.Remote, clientOpts ClientOptions) {
			remoteOpts = options.Remote{}
			_, err := NewSSHClient(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHClientFailsWithEmptyClientOptions": func(t *testing.T, remoteOpts options.Remote, clientOpts ClientOptions) {
			clientOpts = ClientOptions{}
			_, err := NewSSHClient(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHClientSucceedsWithPopulatedOptions": func(t *testing.T, remoteOpts options.Remote, clientOpts ClientOptions) {
			client, err := NewSSHClient(remoteOpts, clientOpts, false)
			require.NoError(t, err)
			assert.NotNil(t, client)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, mockRemoteOptions(), mockClientOptions())
		})
	}
}

func TestSSHClient(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager){
		"VerifyBaseFixtureFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			opts := options.Create{
				Args: []string{"foo", "bar"},
			}
			proc, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
			assert.Nil(t, proc)
		},
		"CreateProcessPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			opts := options.Create{
				Args: []string{"foo", "bar"},
			}
			info := jasper.ProcessInfo{
				ID:        "the_created_process",
				IsRunning: true,
				Options:   opts,
			}

			inputChecker := options.Create{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CreateProcessCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            info,
				},
			)

			proc, err := client.CreateProcess(ctx, &opts)
			require.NoError(t, err)

			assert.Equal(t, opts, inputChecker)

			sshProc, ok := proc.(*sshProcess)
			require.True(t, ok)

			assert.Equal(t, info, sshProc.info)
		},
		"CreateProcessFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			opts := options.Create{Args: []string{"foo", "bar"}}
			_, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"CreateProcessFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CreateProcessCommand},
				nil,
				invalidResponse(),
			)
			opts := options.Create{
				Args: []string{"foo", "bar"},
			}
			_, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"RunCommandPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := options.Command{}

			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CreateCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			cmd := []string{"echo", "foo"}
			require.NoError(t, client.CreateCommand(ctx).Add(cmd).Run(ctx))

			require.Len(t, inputChecker.Commands, 1)
			assert.Equal(t, cmd, inputChecker.Commands[0])
		},
		"RunCommandFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			assert.Error(t, client.CreateCommand(ctx).Add([]string{"echo", "foo"}).Run(ctx))
		},
		"RunCommandFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CreateCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.CreateCommand(ctx).Add([]string{"echo", "foo"}).Run(ctx))
		},
		"RegisterFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			assert.Error(t, client.Register(ctx, &mock.Process{}))
		},
		"ListPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			runningInfo := jasper.ProcessInfo{
				ID:        "running",
				IsRunning: true,
			}
			successfulInfo := jasper.ProcessInfo{
				ID:         "successful",
				Complete:   true,
				Successful: true,
			}

			inputChecker := FilterInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, ListCommand},
				&inputChecker,
				&InfosResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Infos:           []jasper.ProcessInfo{runningInfo, successfulInfo},
				},
			)
			filter := options.All
			procs, err := client.List(ctx, filter)
			require.NoError(t, err)
			assert.Equal(t, filter, inputChecker.Filter)

			runningFound := false
			successfulFound := false
			for _, proc := range procs {
				sshProc, ok := proc.(*sshProcess)
				require.True(t, ok)
				if sshProc.info.ID == runningInfo.ID {
					runningFound = true
				}
				if sshProc.info.ID == successfulInfo.ID {
					successfulFound = true
				}
			}
			assert.True(t, runningFound)
			assert.True(t, successfulFound)
		},
		"ListFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			_, err := client.List(ctx, options.All)
			assert.Error(t, err)
		},
		"ListFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, ListCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.List(ctx, options.All)
			assert.Error(t, err)
		},
		"GroupPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			info := jasper.ProcessInfo{
				ID:        "running",
				IsRunning: true,
			}

			inputChecker := TagInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GroupCommand},
				&inputChecker,
				&InfosResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Infos:           []jasper.ProcessInfo{info},
				},
			)
			tag := "foo"
			procs, err := client.Group(ctx, tag)
			require.NoError(t, err)
			assert.Equal(t, tag, inputChecker.Tag)

			require.Len(t, procs, 1)
			sshProc, ok := procs[0].(*sshProcess)
			require.True(t, ok)
			assert.Equal(t, info, sshProc.info)
		},
		"GroupFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			_, err := client.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GroupFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GroupCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GetPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			id := "foo"
			info := jasper.ProcessInfo{
				ID: id,
			}
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GetCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            info,
				},
			)
			proc, err := client.Get(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, id, inputChecker.ID)

			sshProc, ok := proc.(*sshProcess)
			require.True(t, ok)
			assert.Equal(t, info, sshProc.info)
		},
		"GetFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"GetFailsWIthInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GetCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"ClearPasses": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, ClearCommand},
				nil,
				&struct{}{},
			)
			client.Clear(ctx)
		},
		"ClosePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CloseCommand},
				nil,
				makeOutcomeResponse(nil),
			)
			require.NoError(t, client.Close(ctx))
		},
		"CloseFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			assert.Error(t, client.Close(ctx))
		},
		"CloseFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CloseCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.Close(ctx))
		},
		"CloseConnectionPasses": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			assert.NoError(t, client.CloseConnection())
		},
		"ConfigureCachePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := options.Cache{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, ConfigureCacheCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := options.Cache{PruneDelay: 10, MaxSize: 100}
			require.NoError(t, client.ConfigureCache(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"ConfigureCacheFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, ConfigureCacheCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.ConfigureCache(ctx, options.Cache{}))
		},
		"ConfigureCacheFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			assert.Error(t, client.ConfigureCache(ctx, options.Cache{}))
		},
		"DownloadFilePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := options.Download{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadFileCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := options.Download{URL: "https://example.com", Path: "/foo"}
			require.NoError(t, client.DownloadFile(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"DownloadFileFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadFileCommand},
				nil,
				invalidResponse(),
			)
			opts := options.Download{URL: "https://example.com", Path: "/foo"}
			assert.Error(t, client.DownloadFile(ctx, opts))
		},
		"DownloadFileFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			opts := options.Download{URL: "https://example.com", Path: "/foo"}
			assert.Error(t, client.DownloadFile(ctx, opts))
		},
		"DownloadMongoDBPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := options.MongoDBDownload{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadMongoDBCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := testutil.ValidMongoDBDownloadOptions()
			opts.Path = "/foo"
			require.NoError(t, client.DownloadMongoDB(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"DownloadMongoDBFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadMongoDBCommand},
				nil,
				invalidResponse(),
			)
			opts := testutil.ValidMongoDBDownloadOptions()
			opts.Path = "/foo"
			assert.Error(t, client.DownloadMongoDB(ctx, opts))
		},
		"DownloadMongoDBFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			opts := testutil.ValidMongoDBDownloadOptions()
			opts.Path = "/foo"
			assert.Error(t, client.DownloadMongoDB(ctx, opts))
		},
		"GetLogStreamPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := LogStreamInput{}
			resp := &LogStreamResponse{
				LogStream:       jasper.LogStream{Logs: []string{"foo"}, Done: true},
				OutcomeResponse: *makeOutcomeResponse(nil),
			}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetLogStreamCommand},
				&inputChecker,
				resp,
			)
			id := "foo"
			count := 10
			logs, err := client.GetLogStream(ctx, id, count)
			require.NoError(t, err)

			assert.Equal(t, id, inputChecker.ID)
			assert.Equal(t, count, inputChecker.Count)

			assert.Equal(t, logs, resp.LogStream)
		},
		"GetLogStreamFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetLogStreamCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.GetLogStream(ctx, "foo", 10)
			assert.Error(t, err)
		},
		"GetLogStreamFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			_, err := client.GetLogStream(ctx, "foo", 10)
			assert.Error(t, err)
		},
		"GetBuildloggerURLsPassesWithValidInput": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			resp := &BuildloggerURLsResponse{URLs: []string{"bar"}, OutcomeResponse: *makeOutcomeResponse(nil)}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetBuildloggerURLsCommand},
				&inputChecker,
				resp,
			)
			id := "foo"
			urls, err := client.GetBuildloggerURLs(ctx, id)
			require.NoError(t, err)

			assert.Equal(t, id, inputChecker.ID)

			assert.Equal(t, resp.URLs, urls)
		},
		"GetBuildloggerURLsFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetBuildloggerURLsCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
		},
		"GetBuildloggerURLsFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			_, err := client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
		},
		"SignalEventPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := EventInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, SignalEventCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			name := "foo"
			require.NoError(t, client.SignalEvent(ctx, name))
			assert.Equal(t, name, inputChecker.Name)
		},
		"SignalEventFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, SignalEventCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.SignalEvent(ctx, "foo"))
		},
		"SignalEventFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			assert.Error(t, client.SignalEvent(ctx, "foo"))
		},
		"WriteFileSucceeds": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			inputChecker := options.WriteFile{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, WriteFileCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)

			opts := options.WriteFile{Path: filepath.Join(buildDir(t), "write_file"), Content: []byte("foo")}
			require.NoError(t, client.WriteFile(ctx, opts))
		},
		"WriteFileFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, WriteFileCommand},
				nil,
				invalidResponse(),
			)
			opts := options.WriteFile{Path: filepath.Join(buildDir(t), "write_file"), Content: []byte("foo")}
			assert.Error(t, client.WriteFile(ctx, opts))
		},
		"WriteFileFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			baseManager.FailCreate = true
			opts := options.WriteFile{Path: filepath.Join(buildDir(t), "write_file"), Content: []byte("foo")}
			assert.Error(t, client.WriteFile(ctx, opts))
		},
		"ScriptingReturnsFromCache": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {
			opts := &options.ScriptingPython{
				VirtualEnvPath: "build",
				Packages:       []string{"pymongo"},
			}
			env, err := scripting.NewHarness(baseManager, opts)
			require.NoError(t, err)
			require.NoError(t, client.shCache.Add(env.ID(), env))

			sh, err := client.CreateScripting(ctx, opts)
			require.NoError(t, err)
			require.NotNil(t, sh)
			assert.Equal(t, env.ID(), sh.ID())
		},
		// "": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *mock.Manager) {},
	} {
		t.Run(testName, func(t *testing.T) {
			client, err := NewSSHClient(mockRemoteOptions(), mockClientOptions(), false)
			require.NoError(t, err)
			sshClient, ok := client.(*sshClient)
			require.True(t, ok)

			mockManager := &mock.Manager{}
			sshClient.manager = jasper.Manager(mockManager)

			tctx, cancel := context.WithTimeout(ctx, testutil.TestTimeout)
			defer cancel()

			testCase(tctx, t, sshClient, mockManager)
		})
	}
}

// makeCreateFunc creates the function for the mock manager that reads the
// standard input (if the command accepts JSON input from standard input and
// inputChecker is non-nil) and verifies that it can be unmarshaled into the
// inputChecker for verification by the caller, verifies that the
// expectedClientSubcommand is the CLI command that is being run, and writes the
// expectedResponse back to the user.
func makeCreateFunc(t *testing.T, client *sshClient, expectedClientSubcommand []string, inputChecker interface{}, expectedResponse interface{}) func(*options.Create) mock.Process {
	return func(opts *options.Create) mock.Process {
		if opts.StandardInputBytes != nil && inputChecker != nil {
			input, err := ioutil.ReadAll(bytes.NewBuffer(opts.StandardInputBytes))
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(input, inputChecker))
		}

		cliCommand := strings.Join(client.opts.buildCommand(expectedClientSubcommand...), " ")
		assert.Equal(t, cliCommand, strings.Join(opts.Args, " "))
		require.NotNil(t, expectedResponse)
		require.NoError(t, writeOutput(opts.Output.Output, expectedResponse))
		return mock.Process{}
	}
}

func invalidResponse() interface{} {
	return &struct{}{}
}

func mockRemoteOptions() options.Remote {
	opts := options.Remote{}
	opts.User = "user"
	opts.Host = "localhost"
	opts.Port = 12345
	opts.Password = "abc123"
	return opts
}

func mockClientOptions() ClientOptions {
	return ClientOptions{
		BinaryPath: "binary",
		Type:       RPCService,
	}
}
