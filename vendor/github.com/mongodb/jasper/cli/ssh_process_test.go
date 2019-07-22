package cli

import (
	"context"
	"syscall"
	"testing"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSSHProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager){
		"VerifyFixture": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			assert.Equal(t, jasper.ProcessInfo{ID: "foo"}, proc.info)
		},
		"InfoPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:        proc.ID(),
				IsRunning: true,
			}

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, InfoCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            info,
				},
			)

			assert.Equal(t, info, proc.Info(ctx))
			assert.Equal(t, proc.ID(), inputChecker.ID)
		},
		"InfoWithCompletedProcessChecksInMemory": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:       proc.ID(),
				Complete: true,
			}
			proc.info = info

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, InfoCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            jasper.ProcessInfo{ID: "bar"},
				},
			)

			assert.Equal(t, info, proc.Info(ctx))
			assert.Empty(t, inputChecker.ID)
		},
		"RunningPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:        proc.ID(),
				IsRunning: true,
			}

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, RunningCommand},
				&inputChecker,
				&RunningResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Running:         info.IsRunning,
				},
			)

			assert.Equal(t, info.IsRunning, proc.Running(ctx))
			assert.Equal(t, proc.ID(), inputChecker.ID)
		},
		"RunningWithCompletedProcessChecksInMemory": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:       proc.ID(),
				Complete: true,
			}
			proc.info = info

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, RunningCommand},
				&inputChecker,
				&RunningResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Running:         true,
				},
			)

			assert.False(t, proc.Running(ctx))
			assert.Empty(t, inputChecker.ID)
		},
		"CompletePassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:       proc.ID(),
				Complete: true,
			}

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, CompleteCommand},
				&inputChecker,
				&CompleteResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Complete:        info.Complete,
				},
			)
			assert.Equal(t, info.Complete, proc.Complete(ctx))
			assert.Equal(t, proc.ID(), inputChecker.ID)
		},
		"CompleteWithCompletedProcessChecksInMemory": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:       proc.ID(),
				Complete: true,
			}
			proc.info = info

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, CompleteCommand},
				&inputChecker,
				&CompleteResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Complete:        false,
				},
			)

			assert.True(t, proc.Complete(ctx))
			assert.Empty(t, inputChecker.ID)
		},
		"RespawnPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			info := jasper.ProcessInfo{
				ID:        proc.ID(),
				IsRunning: true,
			}

			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, RespawnCommand},
				&inputChecker,
				&InfoResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Info:            info,
				},
			)

			newProc, err := proc.Respawn(ctx)

			require.NoError(t, err)
			assert.Equal(t, proc.ID(), inputChecker.ID)

			newSSHProc, ok := newProc.(*sshProcess)
			require.True(t, ok)
			require.NoError(t, err)
			assert.Equal(t, info, newSSHProc.info)
		},
		"RespawnFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, RespawnCommand},
				&inputChecker,
				&struct{}{},
			)

			_, err := proc.Respawn(ctx)
			assert.Error(t, err)
			assert.Equal(t, proc.ID(), inputChecker.ID)
		},
		"SignalPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := SignalInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, SignalCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)

			sig := syscall.SIGINT
			require.NoError(t, proc.Signal(ctx, sig))
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.EqualValues(t, sig, inputChecker.Signal)
		},
		"SignalFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := SignalInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, SignalCommand},
				&inputChecker,
				&struct{}{},
			)

			sig := syscall.SIGINT
			assert.Error(t, proc.Signal(ctx, sig))
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.EqualValues(t, sig, inputChecker.Signal)
		},
		"WaitPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			expectedExitCode := 1
			expectedWaitErr := "foo"
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, WaitCommand},
				&inputChecker,
				&WaitResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					ExitCode:        expectedExitCode,
					Error:           expectedWaitErr,
				},
			)

			exitCode, err := proc.Wait(ctx)
			assert.Equal(t, proc.ID(), inputChecker.ID)
			require.Error(t, err)
			assert.Contains(t, err.Error(), expectedWaitErr)
			assert.Equal(t, expectedExitCode, exitCode)
		},
		"WaitFailsWithInvalidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, WaitCommand},
				&inputChecker,
				&struct{}{},
			)

			exitCode, err := proc.Wait(ctx)
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.Error(t, err)
			assert.NotZero(t, exitCode)
		},
		"RegisterTriggerFails": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			assert.Error(t, proc.RegisterTrigger(ctx, func(jasper.ProcessInfo) {}))
		},
		"RegisterSignalTriggerFails": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			assert.Error(t, proc.RegisterSignalTrigger(ctx, func(jasper.ProcessInfo, syscall.Signal) bool { return false }))
		},
		"RegisterSignalTriggerIDPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := SignalTriggerIDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, RegisterSignalTriggerIDCommand},
				&inputChecker,
				makeOutcomeResponse(nil),
			)

			sigID := jasper.SignalTriggerID("foo")
			require.NoError(t, proc.RegisterSignalTriggerID(ctx, sigID))
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.Equal(t, sigID, inputChecker.SignalTriggerID)
		},
		"TagPasses": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := TagIDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, TagCommand},
				&inputChecker,
				&struct{}{},
			)

			tag := "bar"
			proc.Tag(tag)
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.Equal(t, tag, inputChecker.Tag)
		},
		"GetTagsPassesWithValidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			tag := "bar"
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, GetTagsCommand},
				&inputChecker,
				&TagsResponse{
					OutcomeResponse: *makeOutcomeResponse(nil),
					Tags:            []string{tag},
				},
			)

			tags := proc.GetTags()
			assert.Equal(t, proc.ID(), inputChecker.ID)
			require.Len(t, tags, 1)
			assert.Equal(t, tag, tags[0])
		},
		"GetTagsEmptyWithInvalidResponse": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, GetTagsCommand},
				&inputChecker,
				&TagsResponse{
					OutcomeResponse: *makeOutcomeResponse(errors.New("foo")),
				},
			)

			tags := proc.GetTags()
			assert.Equal(t, proc.ID(), inputChecker.ID)
			assert.Empty(t, tags)
		},
		"ResetTagsPasses": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {
			inputChecker := IDInput{}
			baseManager.Create = makeCreateFunc(
				t, manager,
				[]string{ProcessCommand, ResetTagsCommand},
				&inputChecker,
				&struct{}{},
			)
			proc.ResetTags()
			assert.Equal(t, proc.ID(), inputChecker.ID)
		},
		// "": func(ctx context.Context, t *testing.T, proc *sshProcess, manager *sshClient, baseManager *jasper.MockManager) {},
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

			proc, err := newSSHProcess(sshClient.runClientCommand, jasper.ProcessInfo{ID: "foo"})
			require.NoError(t, err)
			sshProc, ok := proc.(*sshProcess)
			require.True(t, ok)

			testCase(tctx, t, sshProc, sshClient, mockManager)
		})
	}
}
