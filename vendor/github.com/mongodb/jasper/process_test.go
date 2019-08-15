package jasper

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := GetHTTPClient()
	defer PutHTTPClient(httpClient)

	for cname, makeProc := range map[string]ProcessConstructor{
		"BlockingNoLock":   newBlockingProcess,
		"BlockingWithLock": makeLockingProcess(newBlockingProcess),
		"BasicNoLock":      newBasicProcess,
		"BasicWithLock":    makeLockingProcess(newBasicProcess),
		"REST": func(ctx context.Context, opts *CreateOptions) (Process, error) {
			_, port, err := startRESTService(ctx, httpClient)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client := &restClient{
				prefix: fmt.Sprintf("http://localhost:%d/jasper/v1", port),
				client: httpClient,
			}

			return client.CreateProcess(ctx, opts)
		},
	} {
		t.Run(cname, func(t *testing.T) {
			for name, testCase := range map[string]func(context.Context, *testing.T, *CreateOptions, ProcessConstructor){
				"WithPopulatedArgsCommandCreationPasses": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					assert.NotZero(t, opts.Args)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.NotNil(t, proc)
				},
				"ErrorToCreateWithInvalidArgs": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts.Args = []string{}
					proc, err := makep(ctx, opts)
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"WithCanceledContextProcessCreationFails": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("context cancellation in test also stops REST service")
					}
					pctx, pcancel := context.WithCancel(ctx)
					pcancel()
					proc, err := makep(pctx, opts)
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
					defer pcancel()
					startAt := time.Now()
					opts = sleepCreateOpts(20)
					proc, err := makep(pctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					time.Sleep(5 * time.Millisecond) // let time pass...
					assert.False(t, proc.Info(ctx).Successful)
					assert.True(t, time.Since(startAt) < 20*time.Second)
				},
				"ProcessLacksTagsByDefault": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					tags := proc.GetTags()
					assert.Empty(t, tags)
				},
				"ProcessTagsPersist": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts.Tags = []string{"foo"}
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					tags := proc.GetTags()
					assert.Contains(t, tags, "foo")
				},
				"InfoHasMatchingID": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					require.NoError(t, err)
					assert.Equal(t, proc.ID(), proc.Info(ctx).ID)
				},
				"ResetTags": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					proc.Tag("foo")
					assert.Contains(t, proc.GetTags(), "foo")
					proc.ResetTags()
					assert.Len(t, proc.GetTags(), 0)
				},
				"TagsAreSetLike": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					for i := 0; i < 10; i++ {
						proc.Tag("foo")
					}

					assert.Len(t, proc.GetTags(), 1)
					proc.Tag("bar")
					assert.Len(t, proc.GetTags(), 2)
				},
				"CompleteIsTrueAfterWait": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					time.Sleep(10 * time.Millisecond) // give the process time to start background machinery
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.True(t, proc.Complete(ctx))
				},
				"WaitReturnsWithCanceledContext": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
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
				"RegisterTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterTrigger(ctx, nil))
				},
				"RegisterSignalTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTrigger(ctx, nil))
				},
				"RegisterSignalTriggerErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTrigger(ctx, func(_ ProcessInfo, _ syscall.Signal) bool { return false }))
				},
				"RegisterSignalTriggerIDErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
				},
				"RegisterSignalTriggerIDFailsWithInvalidTriggerID": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts = sleepCreateOpts(3)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTriggerID(ctx, SignalTriggerID("foo")))
				},
				"RegisterSignalTriggerIDPassesWithValidTriggerID": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts = sleepCreateOpts(3)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.NoError(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
				},
				"DefaultTriggerSucceeds": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}
					proc, err := makep(ctx, opts)
					assert.NoError(t, err)
					assert.NoError(t, proc.RegisterTrigger(ctx, makeDefaultTrigger(ctx, nil, opts, "foo")))
				},
				"OptionsCloseTriggerRegisteredByDefault": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}
					count := 0
					countIncremented := make(chan bool, 1)
					opts.closers = append(opts.closers, func() (_ error) {
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
				"SignalTriggerRunsBeforeSignal": func(ctx context.Context, t *testing.T, _ *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote signal triggers are not supported on rest processes")
					}
					opts := yesCreateOpts(0)
					proc, err := makep(ctx, &opts)
					require.NoError(t, err)

					expectedSig := syscall.SIGKILL
					assert.NoError(t, proc.RegisterSignalTrigger(ctx, func(info ProcessInfo, actualSig syscall.Signal) bool {
						assert.Equal(t, expectedSig, actualSig)
						assert.True(t, info.IsRunning)
						assert.False(t, info.Complete)
						return false
					}))
					proc.Signal(ctx, expectedSig)

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
				"SignalTriggerCanSkipSignal": func(ctx context.Context, t *testing.T, _ *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote signal triggers are not supported on rest processes")
					}
					opts := yesCreateOpts(0)
					proc, err := makep(ctx, &opts)
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
				"ProcessLogDefault": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}

					file, err := ioutil.TempFile("build", "out.txt")
					require.NoError(t, err)
					defer os.Remove(file.Name())
					info, err := file.Stat()
					assert.NoError(t, err)
					assert.Zero(t, info.Size())

					opts.Output.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
					opts.Args = []string{"echo", "foobar"}

					proc, err := makep(ctx, opts)
					assert.NoError(t, err)

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
				},
				"ProcessWritesToLog": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}

					file, err := ioutil.TempFile("build", "out.txt")
					require.NoError(t, err)
					defer os.Remove(file.Name())
					info, err := file.Stat()
					assert.NoError(t, err)
					assert.Zero(t, info.Size())

					opts.Output.Loggers = []Logger{Logger{Type: LogFile, Options: LogOptions{FileName: file.Name(), Format: LogFormatPlain}}}
					opts.Args = []string{"echo", "foobar"}

					proc, err := makep(ctx, opts)
					assert.NoError(t, err)

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					// File is not guaranteed to be written once Wait() returns and closers begin executing,
					// so wait for file to be non-empty.
					fileWrite := make(chan bool)
					go func() {
						done := false
						for !done {
							info, err = file.Stat()
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
						assert.NoError(t, err)
						assert.NotZero(t, info.Size())
					}
				},
				"ProcessWritesToBufferedLog": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}
					file, err := ioutil.TempFile("build", "out.txt")
					require.NoError(t, err)
					defer os.Remove(file.Name())
					info, err := file.Stat()
					assert.NoError(t, err)
					assert.Zero(t, info.Size())

					opts.Output.Loggers = []Logger{Logger{Type: LogFile, Options: LogOptions{
						FileName: file.Name(),
						BufferOptions: BufferOptions{
							Buffered: true,
						},
						Format: LogFormatPlain,
					}}}
					opts.Args = []string{"echo", "foobar"}

					proc, err := makep(ctx, opts)
					assert.NoError(t, err)
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					fileWrite := make(chan int64)
					go func() {
						for {
							info, err = file.Stat()
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
				"WaitOnRespawnedProcessDoesNotError": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
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
				"RespawnedProcessGivesSameResult": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
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
				"RespawningFinishedProcessIsOK": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
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
				"RespawningRunningProcessIsOK": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts = sleepCreateOpts(2)
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
				"TriggersFireOnRespawnedProcessExit": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("remote triggers are not supported on rest processes")
					}
					count := 0
					opts = sleepCreateOpts(2)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					countIncremented := make(chan bool)
					proc.RegisterTrigger(ctx, func(pInfo ProcessInfo) {
						count++
						countIncremented <- true
					})
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
					newProc.RegisterTrigger(ctx, func(pIfno ProcessInfo) {
						count++
						countIncremented <- true
					})
					time.Sleep(3 * time.Second)

					select {
					case <-ctx.Done():
						assert.Fail(t, "triggers took too long to run")
					case <-countIncremented:
						assert.Equal(t, 2, count)
					}
				},
				"RespawnShowsConsistentStateValues": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts = sleepCreateOpts(2)
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
				"WaitGivesSuccessfulExitCode": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, trueCreateOpts())
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Equal(t, 0, exitCode)
				},
				"WaitGivesFailureExitCode": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, falseCreateOpts())
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					assert.Equal(t, 1, exitCode)
				},
				"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, sleepCreateOpts(100))
					require.NoError(t, err)
					require.NotNil(t, proc)
					sig := syscall.SIGTERM
					proc.Signal(ctx, sig)
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					if runtime.GOOS == "windows" {
						assert.Equal(t, 1, exitCode)
					} else {
						assert.Equal(t, int(sig), exitCode)
					}
				},
				"WaitGivesNegativeOneOnAlternativeError": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					cctx, cancel := context.WithCancel(ctx)
					proc, err := makep(ctx, sleepCreateOpts(100))
					require.NoError(t, err)
					require.NotNil(t, proc)

					var exitCode int
					waitFinished := make(chan bool)
					go func() {
						exitCode, err = proc.Wait(cctx)
						waitFinished <- true
					}()
					cancel()
					select {
					case <-waitFinished:
						assert.Error(t, err)
						assert.Equal(t, -1, exitCode)
					case <-ctx.Done():
						assert.Fail(t, "call to Wait() took too long to finish")
					}
					require.NoError(t, Terminate(ctx, proc)) // Clean up.
				},
				"InfoHasTimeoutWhenProcessTimesOut": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					opts = sleepCreateOpts(100)
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
				"CallingSignalOnDeadProcessDoesError": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					err = proc.Signal(ctx, syscall.SIGTERM)
					require.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), "cannot signal a process that has terminated"))
				},
				"StandardInput": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {
					if cname == "REST" {
						t.Skip("standard input behavior should be tested separately on remote interfaces")
					}
					for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte, output *bytes.Buffer){
						"ReaderSetsProcessStandardInput": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte, output *bytes.Buffer) {
							opts.StandardInput = bytes.NewBuffer(stdin)

							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
						},
						"BytesSetsProcessStandardInput": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte, output *bytes.Buffer) {
							opts.StandardInputBytes = stdin

							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
						},
						"ReaderNotRereadByRespawn": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte, output *bytes.Buffer) {
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
						"BytesCopiedByRespawn": func(ctx context.Context, t *testing.T, opts *CreateOptions, expectedOutput string, stdin []byte, output *bytes.Buffer) {
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
							opts = &CreateOptions{
								Args: []string{"bash", "-s"},
								Output: OutputOptions{
									Output: output,
								},
							}
							expectedOutput := "foobar"
							stdin := []byte("echo " + expectedOutput)
							subTestCase(ctx, t, opts, expectedOutput, stdin, output)
						})
					}
				},
				// "": func(ctx context.Context, t *testing.T, opts *CreateOptions, makep ProcessConstructor) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, processTestTimeout)
					defer cancel()

					opts := &CreateOptions{Args: []string{"ls"}}
					testCase(tctx, t, opts, makeProc)
				})
			}
		})
	}
}
