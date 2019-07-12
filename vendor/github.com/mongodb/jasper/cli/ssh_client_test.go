package cli

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSSHClient(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions){
		"NewSSHClientFailsWithEmptyRemoteOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
			remoteOpts = jasper.RemoteOptions{}
			_, err := NewSSHClient(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHClientFailsWithEmptyClientOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
			clientOpts = ClientOptions{}
			_, err := NewSSHClient(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHClientSucceedsWithPopulatedOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
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

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager){
		"VerifyBaseFixtureFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			proc, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
			assert.Nil(t, proc)
		},
		"CreateProcessPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			info := jasper.ProcessInfo{
				ID:        "the_created_process",
				IsRunning: true,
				Options:   opts,
			}

			inputChecker := jasper.CreateOptions{}
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
		"CreateProcessFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			opts := jasper.CreateOptions{Args: []string{"foo", "bar"}}
			_, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"CreateProcessFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CreateProcessCommand},
				nil,
				invalidResponse(),
			)
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			_, err := client.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"RegisterFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			assert.Error(t, client.Register(ctx, &jasper.MockProcess{}))
		},
		"ListPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
			filter := jasper.All
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
		"ListFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := client.List(ctx, jasper.All)
			assert.Error(t, err)
		},
		"ListFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, ListCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.List(ctx, jasper.All)
			assert.Error(t, err)
		},
		"GroupPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
		"GroupFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := client.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GroupFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GroupCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GetPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
		"GetFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"GetFailsWIthInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, GetCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"ClearPasses": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, ClearCommand},
				nil,
				&struct{}{},
			)
			client.Clear(ctx)
		},
		"ClosePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CloseCommand},
				nil,
				makeOutcomeResponse(nil),
			)
			require.NoError(t, client.Close(ctx))
		},
		"CloseFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			assert.Error(t, client.Close(ctx))
		},
		"CloseFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ManagerCommand, CloseCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.Close(ctx))
		},
		"CloseConnectionFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			assert.Error(t, client.CloseConnection())
		},
		"ConfigureCachePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			inputChecker := jasper.CacheOptions{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, ConfigureCacheCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := jasper.CacheOptions{PruneDelay: 10, MaxSize: 100}
			require.NoError(t, client.ConfigureCache(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"ConfigureCacheFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, ConfigureCacheCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.ConfigureCache(ctx, jasper.CacheOptions{}))
		},
		"ConfigureCacheFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			assert.Error(t, client.ConfigureCache(ctx, jasper.CacheOptions{}))
		},
		"DownloadFilePassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			inputChecker := jasper.DownloadInfo{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadFileCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := jasper.DownloadInfo{URL: "https://example.com", Path: "/foo"}
			require.NoError(t, client.DownloadFile(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"DownloadFileFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadFileCommand},
				nil,
				invalidResponse(),
			)
			opts := jasper.DownloadInfo{URL: "https://example.com", Path: "/foo"}
			assert.Error(t, client.DownloadFile(ctx, opts))
		},
		"DownloadFileFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			opts := jasper.DownloadInfo{URL: "https://example.com", Path: "/foo"}
			assert.Error(t, client.DownloadFile(ctx, opts))
		},
		"DownloadMongoDBPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			inputChecker := jasper.MongoDBDownloadOptions{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadMongoDBCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)
			opts := validMongoDBDownloadOptions()
			opts.Path = "/foo"
			require.NoError(t, client.DownloadMongoDB(ctx, opts))

			assert.Equal(t, opts, inputChecker)
		},
		"DownloadMongoDBFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, DownloadMongoDBCommand},
				nil,
				invalidResponse(),
			)
			opts := validMongoDBDownloadOptions()
			opts.Path = "/foo"
			assert.Error(t, client.DownloadMongoDB(ctx, opts))
		},
		"DownloadMongoDBFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			opts := validMongoDBDownloadOptions()
			opts.Path = "/foo"
			assert.Error(t, client.DownloadMongoDB(ctx, opts))
		},
		"GetLogStreamPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
		"GetLogStreamFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetLogStreamCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.GetLogStream(ctx, "foo", 10)
			assert.Error(t, err)
		},
		"GetLogStreamFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := client.GetLogStream(ctx, "foo", 10)
			assert.Error(t, err)
		},
		"GetBuildloggerURLsPassesWithValidInput": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
		"GetBuildloggerURLsFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, GetBuildloggerURLsCommand},
				nil,
				invalidResponse(),
			)
			_, err := client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
		},
		"GetBuildloggerURLsFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := client.GetBuildloggerURLs(ctx, "foo")
			assert.Error(t, err)
		},
		"SignalEventPassesWithValidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
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
		"SignalEventFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{RemoteCommand, SignalEventCommand},
				nil,
				invalidResponse(),
			)
			assert.Error(t, client.SignalEvent(ctx, "foo"))
		},
		"SignalEventFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			assert.Error(t, client.SignalEvent(ctx, "foo"))
		},
		// "": func(ctx context.Context, t *testing.T, client *sshClient, baseManager *jasper.MockManager) {},
	} {
		t.Run(testName, func(t *testing.T) {
			client, err := NewSSHClient(mockRemoteOptions(), mockClientOptions(), false)
			require.NoError(t, err)
			sshClient, ok := client.(*sshClient)
			require.True(t, ok)

			mockManager := &jasper.MockManager{}
			sshClient.manager = jasper.Manager(mockManager)

			tctx, cancel := context.WithTimeout(ctx, testTimeout)
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
func makeCreateFunc(t *testing.T, client *sshClient, expectedClientSubcommand []string, inputChecker interface{}, expectedResponse interface{}) func(*jasper.CreateOptions) jasper.MockProcess {
	return func(opts *jasper.CreateOptions) jasper.MockProcess {
		if opts.StandardInput != nil && inputChecker != nil {
			input, err := ioutil.ReadAll(opts.StandardInput)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(input, inputChecker))
		}

		cliCommand := strings.Join(client.opts.buildCommand(expectedClientSubcommand...), " ")
		cliCommandFound := false
		for _, arg := range opts.Args {
			if noWhitespace(arg) == noWhitespace(cliCommand) {
				cliCommandFound = true
				break
			}
		}
		assert.True(t, cliCommandFound)

		require.True(t, expectedResponse != nil)
		require.NoError(t, writeOutput(opts.Output.Output, expectedResponse))
		return jasper.MockProcess{}
	}
}

func invalidResponse() interface{} {
	return &struct{}{}
}

func mockRemoteOptions() jasper.RemoteOptions {
	return jasper.RemoteOptions{
		User: "user",
		Host: "localhost",
	}
}

func mockClientOptions() ClientOptions {
	return ClientOptions{
		BinaryPath: "binary",
		Type:       RPCService,
	}
}
