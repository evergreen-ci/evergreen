package jasper

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for pname, makeProc := range map[string]ProcessConstructor{
		"BlockingNoLock":   newBlockingProcess,
		"BlockingWithLock": makeLockingProcess(newBlockingProcess),
		"BasicNoLock":      newBasicProcess,
		"BasicWithLock":    makeLockingProcess(newBasicProcess),
	} {
		t.Run(pname, func(t *testing.T) {
			for optsTestName, modifyOpts := range map[string]func(*options.Create) *options.Create{
				"Local": func(opts *options.Create) *options.Create { return opts },
				"Docker": func(opts *options.Create) *options.Create {
					image := os.Getenv("DOCKER_IMAGE")
					if image == "" {
						image = testutil.DefaultDockerImage
					}
					opts.Docker = &options.Docker{
						Image: image,
					}
					return opts
				},
			} {
				if testutil.IsDockerCase(optsTestName) {
					testutil.SkipDockerIfUnsupported(t)
					// TODO (MAKE-1300): remove these lines that clean up docker
					// containers and replace with (Process).Close().
					defer func() {
						client, err := client.NewClientWithOpts(client.WithAPIVersionNegotiation())
						require.NoError(t, err)
						containers, err := client.ContainerList(ctx, types.ContainerListOptions{All: true})
						require.NoError(t, err)
						for _, container := range containers {
							grip.Error(errors.Wrap(client.ContainerRemove(ctx, container.ID, types.ContainerRemoveOptions{Force: true}), "problem cleaning up container"))
						}
					}()
				}

				t.Run(optsTestName, func(t *testing.T) {
					for testName, testCase := range map[string]func(context.Context, *testing.T, *options.Create, ProcessConstructor){
						"WithPopulatedArgsCommandCreationPasses": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							assert.NotZero(t, opts.Args)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.NotNil(t, proc)
						},
						"ErrorToCreateWithInvalidArgs": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							opts.Args = []string{}
							proc, err := makep(ctx, opts)
							assert.Error(t, err)
							assert.Nil(t, proc)
						},
						"WithCanceledContextProcessCreationFails": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							pctx, pcancel := context.WithCancel(ctx)
							pcancel()
							proc, err := makep(pctx, opts)
							assert.Error(t, err)
							assert.Nil(t, proc)
						},
						"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
							defer pcancel()
							startAt := time.Now()
							opts := testutil.SleepCreateOpts(20)
							proc, err := makep(pctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							time.Sleep(5 * time.Millisecond) // let time pass...
							assert.False(t, proc.Info(ctx).Successful)
							assert.True(t, time.Since(startAt) < 20*time.Second)
						},
						"ProcessLacksTagsByDefault": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							tags := proc.GetTags()
							assert.Empty(t, tags)
						},
						"ProcessTagsPersist": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							opts.Tags = []string{"foo"}
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							tags := proc.GetTags()
							assert.Contains(t, tags, "foo")
						},
						"InfoTagsMatchGetTags": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							opts.Tags = []string{"foo"}
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							tags := proc.GetTags()
							assert.Contains(t, tags, "foo")
							assert.Equal(t, tags, proc.Info(ctx).Options.Tags)

							proc.ResetTags()
							tags = proc.GetTags()
							assert.Empty(t, tags)
							assert.Empty(t, proc.Info(ctx).Options.Tags)
						},
						"InfoHasMatchingID": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)
							assert.Equal(t, proc.ID(), proc.Info(ctx).ID)
						},
						"ResetTags": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							proc.Tag("foo")
							assert.Contains(t, proc.GetTags(), "foo")
							proc.ResetTags()
							assert.Len(t, proc.GetTags(), 0)
						},
						"TagsAreSetLike": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							for i := 0; i < 10; i++ {
								proc.Tag("foo")
							}

							assert.Len(t, proc.GetTags(), 1)
							proc.Tag("bar")
							assert.Len(t, proc.GetTags(), 2)
						},
						"CompleteIsTrueAfterWait": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							time.Sleep(10 * time.Millisecond) // give the process time to start background machinery
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.True(t, proc.Complete(ctx))
						},
						"WaitReturnsWithCanceledContext": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							opts.Args = []string{"sleep", "20"}
							pctx, pcancel := context.WithCancel(ctx)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.True(t, proc.Running(ctx))
							assert.NoError(t, err)
							pcancel()
							_, err = proc.Wait(pctx)
							assert.Error(t, err)
						},
						"RegisterTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterTrigger(ctx, nil))
						},
						"RegisterSignalTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTrigger(ctx, nil))
						},
						"RegisterSignalTriggerErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTrigger(ctx, func(_ ProcessInfo, _ syscall.Signal) bool { return false }))
						},
						"RegisterSignalTriggerIDErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
						},
						"RegisterSignalTriggerIDFailsWithInvalidTriggerID": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTriggerID(ctx, SignalTriggerID("foo")))
						},
						"RegisterSignalTriggerIDPassesWithValidTriggerID": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.NoError(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
						},
						"DefaultTriggerSucceeds": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.NoError(t, proc.RegisterTrigger(ctx, makeDefaultTrigger(ctx, nil, opts, "foo")))
						},
						"OptionsCloseTriggerRegisteredByDefault": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							count := 0
							countIncremented := make(chan bool, 1)
							opts.RegisterCloser(func() (_ error) {
								count++
								countIncremented <- true
								close(countIncremented)
								return
							})

							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							select {
							case <-ctx.Done():
								assert.Fail(t, "closers took too long to run")
							case <-countIncremented:
								assert.Equal(t, 1, count)
							}
						},
						"SignalTriggerRunsBeforeSignal": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.SleepCreateOpts(1))
							require.NoError(t, err)

							expectedSig := syscall.SIGKILL
							assert.NoError(t, proc.RegisterSignalTrigger(ctx, func(info ProcessInfo, actualSig syscall.Signal) bool {
								assert.Equal(t, expectedSig, actualSig)
								assert.True(t, info.IsRunning)
								assert.False(t, info.Complete)
								return false
							}))
							assert.NoError(t, proc.Signal(ctx, expectedSig))

							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(expectedSig), exitCode)
							}

							assert.False(t, proc.Running(ctx))
							assert.True(t, proc.Complete(ctx))
						},
						"SignalTriggerCanSkipSignal": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.SleepCreateOpts(1))
							require.NoError(t, err)

							expectedSig := syscall.SIGKILL
							shouldSkipNextTime := true
							assert.NoError(t, proc.RegisterSignalTrigger(ctx, func(info ProcessInfo, actualSig syscall.Signal) bool {
								assert.Equal(t, expectedSig, actualSig)
								skipSignal := shouldSkipNextTime
								shouldSkipNextTime = false
								return skipSignal
							}))

							assert.NoError(t, proc.Signal(ctx, expectedSig))
							assert.True(t, proc.Running(ctx))
							assert.False(t, proc.Complete(ctx))

							assert.NoError(t, proc.Signal(ctx, expectedSig))

							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(expectedSig), exitCode)
							}

							assert.False(t, proc.Running(ctx))
							assert.True(t, proc.Complete(ctx))
						},
						"ProcessLogDefault": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							file, err := ioutil.TempFile(testutil.BuildDirectory(), "out.txt")
							require.NoError(t, err)
							defer func() {
								assert.NoError(t, file.Close())
								assert.NoError(t, os.RemoveAll(file.Name()))
							}()
							info, err := file.Stat()
							require.NoError(t, err)
							assert.Zero(t, info.Size())

							logger := &options.LoggerConfig{}
							require.NoError(t, logger.Set(&options.DefaultLoggerOptions{
								Base: options.BaseOptions{Format: options.LogFormatPlain},
							}))
							opts.Output.Loggers = []*options.LoggerConfig{logger}
							opts.Args = []string{"echo", "foobar"}

							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
						},
						"ProcessWritesToLog": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							file, err := ioutil.TempFile(testutil.BuildDirectory(), "out.txt")
							require.NoError(t, err)
							defer func() {
								assert.NoError(t, file.Close())
								assert.NoError(t, os.RemoveAll(file.Name()))
							}()
							info, err := file.Stat()
							require.NoError(t, err)
							assert.Zero(t, info.Size())

							logger := &options.LoggerConfig{}
							require.NoError(t, logger.Set(&options.FileLoggerOptions{
								Filename: file.Name(),
								Base:     options.BaseOptions{Format: options.LogFormatPlain},
							}))
							opts.Output.Loggers = []*options.LoggerConfig{logger}
							opts.Args = []string{"echo", "foobar"}

							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							assert.NoError(t, err)

							// File is not guaranteed to be written once Wait() returns and closers begin executing,
							// so wait for file to be non-empty.
							fileWrite := make(chan bool)
							go func() {
								done := false
								for !done {
									info, err = file.Stat()
									require.NoError(t, err)
									if info.Size() > 0 {
										done = true
										fileWrite <- done
									}
								}
							}()

							select {
							case <-ctx.Done():
								assert.Fail(t, "file write took too long to complete")
							case <-fileWrite:
								info, err = file.Stat()
								require.NoError(t, err)
								assert.NotZero(t, info.Size())
							}
						},
						"ProcessWritesToBufferedLog": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							file, err := ioutil.TempFile(testutil.BuildDirectory(), "out.txt")
							require.NoError(t, err)
							defer func() {
								assert.NoError(t, file.Close())
								assert.NoError(t, os.RemoveAll(file.Name()))
							}()
							info, err := file.Stat()
							require.NoError(t, err)
							assert.Zero(t, info.Size())

							logger := &options.LoggerConfig{}
							require.NoError(t, logger.Set(&options.FileLoggerOptions{
								Filename: file.Name(),
								Base: options.BaseOptions{
									Buffer: options.BufferOptions{
										Buffered: true,
									},
									Format: options.LogFormatPlain,
								},
							}))
							opts.Output.Loggers = []*options.LoggerConfig{logger}
							opts.Args = []string{"echo", "foobar"}

							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							fileWrite := make(chan int64)
							go func() {
								for {
									info, err = file.Stat()
									require.NoError(t, err)
									if info.Size() > 0 {
										fileWrite <- info.Size()
										break
									}
								}
							}()

							select {
							case <-ctx.Done():
								assert.Fail(t, "file write took too long to complete")
							case size := <-fileWrite:
								assert.NotZero(t, size)
							}
						},
						"WaitOnRespawnedProcessDoesNotError": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							_, err = newProc.Wait(ctx)
							assert.NoError(t, err)
						},
						"RespawnedProcessGivesSameResult": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)
							procExitCode := proc.Info(ctx).ExitCode

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.Equal(t, procExitCode, proc.Info(ctx).ExitCode)
						},
						"RespawningFinishedProcessIsOK": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							require.NotNil(t, newProc)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Info(ctx).Successful)
						},
						"RespawningRunningProcessIsOK": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(2)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							require.NotNil(t, newProc)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Info(ctx).Successful)
						},
						"TriggersFireOnRespawnedProcessExit": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							count := 0
							opts := testutil.SleepCreateOpts(2)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							countIncremented := make(chan bool)
							assert.NoError(t, proc.RegisterTrigger(ctx, func(pInfo ProcessInfo) {
								count++
								countIncremented <- true
							}))
							time.Sleep(3 * time.Second)

							select {
							case <-ctx.Done():
								assert.Fail(t, "triggers took too long to run")
							case <-countIncremented:
								require.Equal(t, 1, count)
							}

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							require.NotNil(t, newProc)
							assert.NoError(t, newProc.RegisterTrigger(ctx, func(pIfno ProcessInfo) {
								count++
								countIncremented <- true
							}))
							time.Sleep(3 * time.Second)

							select {
							case <-ctx.Done():
								assert.Fail(t, "triggers took too long to run")
							case <-countIncremented:
								assert.Equal(t, 2, count)
							}
						},
						"RespawnShowsConsistentStateValues": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(2)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Running(ctx))
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Complete(ctx))
						},
						"WaitGivesSuccessfulExitCode": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.TrueCreateOpts())
							require.NoError(t, err)
							require.NotNil(t, proc)
							exitCode, err := proc.Wait(ctx)
							assert.NoError(t, err)
							assert.Equal(t, 0, exitCode)
						},
						"WaitGivesFailureExitCode": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.FalseCreateOpts())
							require.NoError(t, err)
							require.NotNil(t, proc)
							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							assert.Equal(t, 1, exitCode)
						},
						"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.SleepCreateOpts(100))
							require.NoError(t, err)
							require.NotNil(t, proc)
							sig := syscall.SIGTERM
							assert.NoError(t, proc.Signal(ctx, sig))
							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(sig), exitCode)
							}
						},
						"WaitGivesProperExitCodeOnSignalAbort": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.SleepCreateOpts(100))
							require.NoError(t, err)
							require.NotNil(t, proc)
							sig := syscall.SIGABRT
							assert.NoError(t, proc.Signal(ctx, sig))
							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(sig), exitCode)
							}
						},
						"WaitGivesNegativeOneOnAlternativeError": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testutil.SleepCreateOpts(100))
							require.NoError(t, err)
							require.NotNil(t, proc)

							var exitCode int
							waitFinished := make(chan bool)
							cctx, cancel := context.WithCancel(ctx)
							cancel()
							go func() {
								exitCode, err = proc.Wait(cctx)
								waitFinished <- true
							}()
							select {
							case <-waitFinished:
								assert.Error(t, err)
								assert.Equal(t, -1, exitCode)
							case <-ctx.Done():
								assert.Fail(t, "call to Wait() took too long to finish")
							}
						},
						"InfoHasTimeoutWhenProcessTimesOut": func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							opts := testutil.SleepCreateOpts(100)
							opts.Timeout = time.Second
							opts.TimeoutSecs = 1
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							exitCode, err := proc.Wait(ctx)
							assert.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(syscall.SIGKILL), exitCode)
							}
							assert.True(t, proc.Info(ctx).Timeout)
						},
						"CallingSignalOnDeadProcessDoesError": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							assert.NoError(t, err)

							err = proc.Signal(ctx, syscall.SIGTERM)
							require.Error(t, err)
							assert.True(t, strings.Contains(err.Error(), "cannot signal a process that has terminated"))
						},
						"StandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
							for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer){
								"ReaderSetsProcessStandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInput = bytes.NewBuffer(stdin)

									proc, err := makep(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
								},
								"BytesSetsProcessStandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInputBytes = stdin

									proc, err := makep(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
								},
								"ReaderNotRereadByRespawn": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInput = bytes.NewBuffer(stdin)

									proc, err := makep(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))

									output.Reset()

									newProc, err := proc.Respawn(ctx)
									require.NoError(t, err)

									_, err = newProc.Wait(ctx)
									require.NoError(t, err)

									assert.Empty(t, output.String())

									assert.Equal(t, proc.Info(ctx).Options.StandardInput, newProc.Info(ctx).Options.StandardInput)
								},
								"BytesCopiedByRespawn": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInputBytes = stdin

									proc, err := makep(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))

									output.Reset()

									newProc, err := proc.Respawn(ctx)
									require.NoError(t, err)

									_, err = newProc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
								},
							} {
								t.Run(subTestName, func(t *testing.T) {
									output := &bytes.Buffer{}
									opts = &options.Create{
										Args: []string{"bash", "-s"},
										Output: options.Output{
											Output: output,
										},
									}
									expectedOutput := "foobar"
									stdin := []byte("echo " + expectedOutput)
									subTestCase(ctx, t, opts, expectedOutput, stdin, output)
								})
							}
						},
						// "": func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {},
					} {
						t.Run(testName, func(t *testing.T) {
							tctx, cancel := context.WithTimeout(ctx, testutil.ProcessTestTimeout)
							defer cancel()

							opts := &options.Create{Args: []string{"ls"}}
							opts = modifyOpts(opts)
							testCase(tctx, t, opts, makeProc)
						})
					}
				})
			}
		})
	}
}
