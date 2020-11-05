package cli

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/mongodb/jasper/scripting"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHScriptingHarness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager){
		"SetupPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingSetupCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, sh.Setup(ctx))
		},
		"SetupFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingSetupCommand},
				nil,
				invalidResponse(),
			)

			assert.Error(t, sh.Setup(ctx))
		},
		"RunPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &ScriptingRunInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingRunCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, sh.Run(ctx, []string{"args"}))
		},
		"RunFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingRunCommand},
				nil,
				invalidResponse(),
			)

			assert.Error(t, sh.Run(ctx, []string{"args"}))
		},
		"RunScriptPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &ScriptingRunScriptInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingRunScriptCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, sh.RunScript(ctx, "script"))
		},
		"RunScriptFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingRunScriptCommand},
				nil,
				invalidResponse(),
			)

			assert.Error(t, sh.RunScript(ctx, "script"))
		},
		"BuildPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &ScriptingBuildInput{}
			resp := &ScriptingBuildResponse{
				OutcomeResponse: *makeOutcomeResponse(nil),
				Path:            "path",
			}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingBuildCommand},
				inputChecker,
				resp,
			)

			path, err := sh.Build(ctx, "dir", []string{"args"})
			require.NoError(t, err)
			assert.Equal(t, resp.Path, path)
		},
		"BuildFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingBuildCommand},
				nil,
				invalidResponse(),
			)

			path, err := sh.Build(ctx, "dir", []string{"args"})
			assert.Error(t, err)
			assert.Zero(t, path)
		},
		"TestPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &ScriptingTestInput{}
			resp := &ScriptingTestResponse{
				OutcomeResponse: *makeOutcomeResponse(nil),
				Results: []scripting.TestResult{
					{
						Name:     "name",
						Outcome:  scripting.TestOutcomeSuccess,
						Duration: time.Minute,
					},
				},
			}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingTestCommand},
				inputChecker,
				resp,
			)

			results, err := sh.Test(ctx, "dir")
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Equal(t, resp.Results[0].Name, results[0].Name)
			assert.Equal(t, resp.Results[0].Duration, results[0].Duration)
			assert.Equal(t, resp.Results[0].Outcome, results[0].Outcome)
		},
		"TestFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingTestCommand},
				nil,
				invalidResponse(),
			)

			path, err := sh.Test(ctx, "dir")
			assert.Error(t, err)
			assert.Zero(t, path)
		},
		"CleanupPassesWithValidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			inputChecker := &IDInput{}
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingCleanupCommand},
				inputChecker,
				makeOutcomeResponse(nil),
			)

			assert.NoError(t, sh.Cleanup(ctx))
		},
		"CleanupFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, sh *sshScriptingHarness, client *sshClient, baseManager *mock.Manager) {
			baseManager.Create = makeCreateFunc(
				t, client,
				[]string{ScriptingCommand, ScriptingCleanupCommand},
				nil,
				invalidResponse(),
			)

			assert.Error(t, sh.Cleanup(ctx))
		},
	} {
		t.Run(testName, func(t *testing.T) {
			client, err := NewSSHClient(mockClientOptions(), mockRemoteOptions())
			require.NoError(t, err)
			sshClient, ok := client.(*sshClient)
			require.True(t, ok)

			mockManager := &mock.Manager{}
			sshClient.client.manager = jasper.Manager(mockManager)

			tctx, cancel := context.WithTimeout(ctx, testutil.TestTimeout)
			defer cancel()

			sh := newSSHScriptingHarness(ctx, sshClient.client, "id")
			require.NotNil(t, sh)

			testCase(tctx, t, sh, sshClient, mockManager)
		})
	}
}
