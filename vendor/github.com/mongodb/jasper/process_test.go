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
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for pname, makeProc := range map[string]ProcessConstructor{
		"BlockingProcess": newBlockingProcess,
		"BlockingSynchronizedProcess": func(ctx context.Context, opts *options.Create) (Process, error) {
			opts.Implementation = options.ProcessImplementationBlocking
			return newSynchronizedProcess(ctx, opts)
		},
		"BasicProcess": newBasicProcess,
		"BasicSynchronizedProcess": func(ctx context.Context, opts *options.Create) (Process, error) {
			opts.Implementation = options.ProcessImplementationBasic
			return newSynchronizedProcess(ctx, opts)
		},
	} {
		t.Run(pname, func(t *testing.T) {
			for optsTestName, modifyOpts := range map[string]testoptions.ModifyOpts{
				"LocalExecutor": func(opts *options.Create) *options.Create { return opts },
				"DockerExecutor": func(opts *options.Create) *options.Create {
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

				testCases := append(ProcessTests(), []ProcessTestCase{
					{
						Name: "CanceledContextTimesOutEarly",
						Case: func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
							defer pcancel()
							startAt := time.Now()
							opts := testoptions.SleepCreateOpts(20)
							proc, err := makep(pctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							time.Sleep(5 * time.Millisecond) // let time pass...
							assert.False(t, proc.Info(ctx).Successful)
							assert.True(t, time.Since(startAt) < 20*time.Second)
						},
					},
					{
						Name: "OptionsCloseTriggerRegisteredByDefault",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
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
					},
					{
						Name: "WaitGivesProperExitCodeOnSignalAbort",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
							proc, err := makeProc(ctx, testoptions.SleepCreateOpts(5))
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
					},
					{
						Name: "SignalTriggerRunsBeforeSignal",
						Case: func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testoptions.SleepCreateOpts(1))
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
					},
					{
						Name: "SignalTriggerCanSkipSignal",
						Case: func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							proc, err := makep(ctx, testoptions.SleepCreateOpts(1))
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
					},
					{
						Name: "ProcessLogDefault",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
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
					},
					{
						Name: "ProcessWritesToLogFile",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
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
								for {
									info, err = file.Stat()
									require.NoError(t, err)
									if info.Size() > 0 {
										select {
										case <-ctx.Done():
										case fileWrite <- true:
										}
										return
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
					},
					{
						Name: "ProcessWritesToBufferedLog",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makep ProcessConstructor) {
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
										select {
										case <-ctx.Done():
										case fileWrite <- info.Size():
										}
										return
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
					},
					{
						Name: "TriggersFireOnRespawnedProcessExit",
						Case: func(ctx context.Context, t *testing.T, _ *options.Create, makep ProcessConstructor) {
							count := 0
							opts := testoptions.SleepCreateOpts(2)
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
					},
					{
						Name: "ProcessStandardInput",
						Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
							for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer){
								"ReaderIsSet": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInput = bytes.NewBuffer(stdin)

									proc, err := makeProc(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
								},
								"ReaderNotRereadByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInput = bytes.NewBuffer(stdin)

									proc, err := makeProc(ctx, opts)
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
								"BytesIsSet": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInputBytes = stdin

									proc, err := makeProc(ctx, opts)
									require.NoError(t, err)

									_, err = proc.Wait(ctx)
									require.NoError(t, err)

									assert.Equal(t, expectedOutput, strings.TrimSpace(output.String()))
								},
								"BytesCopiedByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte, output *bytes.Buffer) {
									opts.StandardInputBytes = stdin

									proc, err := makeProc(ctx, opts)
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
					},
				}...)

				t.Run(optsTestName, func(t *testing.T) {
					for _, testCase := range testCases {
						t.Run(testCase.Name, func(t *testing.T) {
							tctx, tcancel := context.WithTimeout(ctx, testutil.ProcessTestTimeout)
							defer tcancel()

							opts := modifyOpts(&options.Create{Args: []string{"ls"}})
							testCase.Case(tctx, t, opts, makeProc)
						})
					}
				})
			}

		})
	}
}
