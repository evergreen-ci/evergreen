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

func TestNewSSHManager(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions){
		"NewSSHManagerFailsWithEmptyRemoteOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
			remoteOpts = jasper.RemoteOptions{}
			_, err := NewSSHManager(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHManagerFailsWithEmptyClientOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
			clientOpts = ClientOptions{}
			_, err := NewSSHManager(remoteOpts, clientOpts, false)
			assert.Error(t, err)
		},
		"NewSSHManagerSucceedsWithPopulatedOptions": func(t *testing.T, remoteOpts jasper.RemoteOptions, clientOpts ClientOptions) {
			manager, err := NewSSHManager(remoteOpts, clientOpts, false)
			require.NoError(t, err)
			assert.NotNil(t, manager)
		},
	} {
		t.Run(testName, func(t *testing.T) {
			testCase(t, mockRemoteOptions(), mockClientOptions())
		})
	}
}

func TestSSHManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager){
		"VerifyBaseFixtureFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			proc, err := manager.CreateProcess(ctx, &opts)
			assert.Error(t, err)
			assert.Nil(t, proc)
		},
		"CreateProcessPassesWithValidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			createdInfo := jasper.ProcessInfo{
				ID:        "the_created_process",
				IsRunning: true,
				Options:   opts,
			}

			inputChecker := jasper.CreateOptions{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, CreateProcessCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            createdInfo,
				},
			)

			proc, err := manager.CreateProcess(ctx, &opts)
			require.NoError(t, err)
			assert.Equal(t, opts, inputChecker)

			_, ok := proc.(*sshProcess)
			require.True(t, ok)
		},
		"CreateProcessFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			opts := jasper.CreateOptions{Args: []string{"foo", "bar"}}
			_, err := manager.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"CreateProcessFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, CreateProcessCommand},
				nil,
				&struct{}{},
			)
			opts := jasper.CreateOptions{
				Args: []string{"foo", "bar"},
			}
			_, err := manager.CreateProcess(ctx, &opts)
			assert.Error(t, err)
		},
		"RegisterFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			assert.Error(t, manager.Register(ctx, &jasper.MockProcess{}))
		},
		"ListPassesWithValidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
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
				t, manager,
				[]string{ManagerCommand, ListCommand},
				&inputChecker,
				&InfosResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Infos:           []jasper.ProcessInfo{runningInfo, successfulInfo},
				},
			)
			filter := jasper.All
			procs, err := manager.List(ctx, filter)
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
		"ListFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := manager.List(ctx, jasper.All)
			assert.Error(t, err)
		},
		"ListFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, ListCommand},
				nil,
				&struct{}{},
			)
			_, err := manager.List(ctx, jasper.All)
			assert.Error(t, err)
		},
		"GroupPassesWithValidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:        "running",
				IsRunning: true,
			}

			inputChecker := TagInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, GroupCommand},
				&inputChecker,
				&InfosResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Infos:           []jasper.ProcessInfo{info},
				},
			)
			tag := "foo"
			procs, err := manager.Group(ctx, tag)
			require.NoError(t, err)
			assert.Equal(t, tag, inputChecker.Tag)

			require.Len(t, procs, 1)
			sshProc, ok := procs[0].(*sshProcess)
			require.True(t, ok)
			assert.Equal(t, info, sshProc.info)
		},
		"GroupFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := manager.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GroupFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, GroupCommand},
				nil,
				&struct{}{},
			)
			_, err := manager.Group(ctx, "foo")
			assert.Error(t, err)
		},
		"GetPassesWithValidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			id := "foo"
			info := jasper.ProcessInfo{
				ID: id,
			}
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, GetCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            info,
				},
			)
			proc, err := manager.Get(ctx, id)
			require.NoError(t, err)
			assert.Equal(t, id, inputChecker.ID)

			sshProc, ok := proc.(*sshProcess)
			require.True(t, ok)
			assert.Equal(t, info, sshProc.info)
		},
		"GetFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			_, err := manager.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"GetFailsWIthInvalidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, GetCommand},
				nil,
				&struct{}{},
			)
			_, err := manager.Get(ctx, "foo")
			assert.Error(t, err)
		},
		"ClearPasses": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, ClearCommand},
				nil,
				&struct{}{},
			)
			manager.Clear(ctx)
		},
		"ClosePassesWithValidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, CloseCommand},
				nil,
				makeOutcomeResponse(nil),
			)
			require.NoError(t, manager.Close(ctx))
		},
		"CloseFailsIfBaseManagerCreateFails": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.FailCreate = true
			assert.Error(t, manager.Close(ctx))
		},
		"CloseFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ManagerCommand, CloseCommand},
				nil,
				&struct{}{},
			)
			assert.Error(t, manager.Close(ctx))
		},
		// "": func(ctx context.Context, t *testing.T, manager *sshManager, baseManager *jasper.MockManager) {},
	} {
		t.Run(testName, func(t *testing.T) {
			mngr, err := NewSSHManager(mockRemoteOptions(), mockClientOptions(), false)
			require.NoError(t, err)
			sshMngr, ok := mngr.(*sshManager)
			require.True(t, ok)

			mockMngr := &jasper.MockManager{}
			sshMngr.manager = jasper.Manager(mockMngr)

			tctx, cancel := context.WithTimeout(ctx, testTimeout)
			defer cancel()

			testCase(tctx, t, sshMngr, mockMngr)
		})
	}
}

// stringsEqualIgnoreWhitespace checks that two strings are equal if whitespace
// is ignored.
func stringsEqualIgnoreWhitespace(s0, s1 string) bool {
	return strings.Join(strings.Fields(s0), "") == strings.Join(strings.Fields(s1), "")
}

// makeCreateFunc creates the function for the mock manager that reads the
// standard input for the JSON input that will go into the CLI into
// inputChecker, verifies that the expectedClientSubcommand is the CLI command
// that is being run, and writes the expectedResponse back to the user.
func makeCreateFunc(t *testing.T, manager *sshManager, expectedClientSubcommand []string, inputChecker interface{}, expectedResponse interface{}) func(*jasper.CreateOptions) jasper.MockProcess {
	return func(opts *jasper.CreateOptions) jasper.MockProcess {
		if opts.StandardInput != nil && inputChecker != nil {
			input, err := ioutil.ReadAll(opts.StandardInput)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(input, inputChecker))
		}

		cliCommand := manager.opts.buildCommand(expectedClientSubcommand...)
		cliCommandFound := false
		for _, arg := range opts.Args {
			if stringsEqualIgnoreWhitespace(arg, strings.Join(cliCommand, " ")) {
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
