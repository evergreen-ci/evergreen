package jasper

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	echo, ls         = "echo", "ls"
	arg1, arg2, arg3 = "ZXZlcmdyZWVu", "aXM=", "c28gY29vbCE="
	lsErrorMsg       = "No such file or directory"
)

type Buffer struct {
	b bytes.Buffer
	sync.RWMutex
}

func (b *Buffer) Read(p []byte) (n int, err error) {
	b.RLock()
	defer b.RUnlock()
	return b.b.Read(p)
}
func (b *Buffer) Write(p []byte) (n int, err error) {
	b.Lock()
	defer b.Unlock()
	return b.b.Write(p)
}
func (b *Buffer) String() string {
	b.RLock()
	defer b.RUnlock()
	return b.b.String()
}

func (b *Buffer) Close() error { return nil }

func verifyCommandAndGetOutput(ctx context.Context, t *testing.T, cmd *Command, run cmdRunFunc, success bool) string {
	var buf bytes.Buffer
	bufCloser := &Buffer{b: buf}

	cmd.SetCombinedWriter(bufCloser)

	if success {
		assert.NoError(t, run(cmd, ctx))
	} else {
		assert.Error(t, run(cmd, ctx))
	}

	return bufCloser.String()
}

func checkOutput(t *testing.T, exists bool, output string, expectedOutputs ...string) {
	for _, expected := range expectedOutputs {
		// TODO: Maybe don't try to be so cheeky with an XOR...
		assert.True(t, exists == strings.Contains(output, expected))
	}
}

type cmdRunFunc func(*Command, context.Context) error

func TestCommandImplementation(t *testing.T) {
	cwd, err := os.Getwd()
	require.NoError(t, err)
	for procType, makep := range map[string]ProcessConstructor{
		"BlockingNoLock": newBasicProcess,
		"BlockingLock":   makeLockingProcess(newBasicProcess),
		"BasicNoLock":    newBasicProcess,
		"BasicLock":      makeLockingProcess(newBasicProcess),
	} {
		t.Run(procType, func(t *testing.T) {
			for runFuncType, runFunc := range map[string]cmdRunFunc{
				"NonParallel": (*Command).Run,
				"Parallel":    (*Command).RunParallel,
			} {
				t.Run(runFuncType, func(t *testing.T) {
					for name, testCase := range map[string]func(context.Context, *testing.T, Command){
						"ValidRunCommandDoesNotError": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.ID(t.Name()).Priority(level.Info).Add(
								[]string{echo, arg1},
							).Directory(cwd)
							assert.NoError(t, runFunc(&cmd, ctx))
						},
						"UnsuccessfulRunCommandErrors": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.ID(t.Name()).Priority(level.Info).Add(
								[]string{ls, arg2},
							).Directory(cwd)
							err := runFunc(&cmd, ctx)
							assert.Error(t, err)
							assert.True(t, strings.Contains(err.Error(), "exit status"))
						},
						"SudoFunctions": func(ctx context.Context, t *testing.T, cmd Command) {
							user := "user"
							sudoUser := "root"

							sudoCmd := "sudo"
							sudoAsCmd := fmt.Sprintf("sudo -u %s", sudoUser)

							cmd1 := strings.Join([]string{echo, arg1}, " ")
							cmd2 := strings.Join([]string{echo, arg2}, " ")

							for commandType, isRemote := range map[string]bool{
								"Remote": true,
								"Local":  false,
							} {
								t.Run(commandType, func(t *testing.T) {
									for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, cmd Command){
										"VerifySudoCmd": func(ctx context.Context, t *testing.T, cmd Command) {
											cmd.Sudo(true)
											assert.Equal(t, strings.Join(cmd.sudoCmd(), " "), sudoCmd)
										},
										"VerifySudoCmdWithUser": func(ctx context.Context, t *testing.T, cmd Command) {
											cmd.SudoAs(sudoUser)
											assert.Equal(t, strings.Join(cmd.sudoCmd(), " "), sudoAsCmd)
										},
										"NoSudo": func(ctx context.Context, t *testing.T, cmd Command) {
											cmd.Append(cmd1)

											allOpts, err := cmd.getCreateOpts(ctx)
											require.NoError(t, err)
											require.Len(t, allOpts, 1)
											args := strings.Join(allOpts[0].Args, " ")

											assert.NotContains(t, args, sudoCmd)
										},
										"Sudo": func(ctx context.Context, t *testing.T, cmd Command) {
											checkArgs := func(args []string, expected string) {
												argsStr := strings.Join(args, " ")
												assert.Contains(t, argsStr, sudoCmd)
												assert.NotContains(t, argsStr, sudoAsCmd)
												assert.Contains(t, argsStr, expected)
											}
											cmd.Sudo(true).Append(cmd1)

											allOpts, err := cmd.getCreateOpts(ctx)
											require.NoError(t, err)
											require.Len(t, allOpts, 1)
											checkArgs(allOpts[0].Args, cmd1)

											cmd.Append(cmd2)
											allOpts, err = cmd.getCreateOpts(ctx)
											require.NoError(t, err)
											require.Len(t, allOpts, 2)

											checkArgs(allOpts[0].Args, cmd1)
											checkArgs(allOpts[1].Args, cmd2)
										},
										"SudoAs": func(ctx context.Context, t *testing.T, cmd Command) {
											cmd.SudoAs(sudoUser).Add([]string{echo, arg1})
											checkArgs := func(args []string, expected string) {
												argsStr := strings.Join(args, " ")
												assert.Contains(t, argsStr, sudoAsCmd)
												assert.Contains(t, argsStr, expected)
											}

											allOpts, err := cmd.getCreateOpts(ctx)
											require.NoError(t, err)
											require.Len(t, allOpts, 1)
											checkArgs(allOpts[0].Args, cmd1)

											cmd.Add([]string{echo, arg2})
											allOpts, err = cmd.getCreateOpts(ctx)
											require.NoError(t, err)
											require.Len(t, allOpts, 2)
											checkArgs(allOpts[0].Args, cmd1)
											checkArgs(allOpts[1].Args, cmd2)
										},
									} {
										t.Run(subTestName, func(t *testing.T) {
											cmd = *NewCommand().ProcConstructor(cmd.makep)
											if isRemote {
												cmd.User(user).Host("localhost")
											}
											subTestCase(ctx, t, cmd)
										})
									}
								})
							}
						},
						"InvalidArgsCommandErrors": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.Add([]string{})
							assert.EqualError(t, runFunc(&cmd, ctx), "cannot have empty args")
						},
						"ZeroSubCommandsIsVacuouslySuccessful": func(ctx context.Context, t *testing.T, cmd Command) {
							assert.NoError(t, runFunc(&cmd, ctx))
						},
						"PreconditionDeterminesExecution": func(ctx context.Context, t *testing.T, cmd Command) {
							for _, precondition := range []func() bool{
								func() bool {
									return true
								},
								func() bool {
									return false
								},
							} {
								t.Run(fmt.Sprintf("%tPrecondition", precondition()), func(t *testing.T) {
									cmd.Prerequisite(precondition).Add([]string{echo, arg1})
									output := verifyCommandAndGetOutput(ctx, t, &cmd, runFunc, true)
									checkOutput(t, precondition(), output, arg1)
								})
							}
						},
						"SingleInvalidSubCommandCausesTotalError": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.ID(t.Name()).Priority(level.Info).Extend(
								[][]string{
									[]string{echo, arg1},
									[]string{ls, arg2},
									[]string{echo, arg3},
								},
							).Directory(cwd)
							assert.Error(t, runFunc(&cmd, ctx))
						},
						"ExecutionFlags": func(ctx context.Context, t *testing.T, cmd Command) {
							numCombinations := int(math.Pow(2, 3))
							for i := 0; i < numCombinations; i++ {
								continueOnError, ignoreError, includeBadCmd := i&1 == 1, i&2 == 2, i&4 == 4

								cmd := NewCommand().Add([]string{echo, arg1}).ProcConstructor(cmd.makep)
								if includeBadCmd {
									cmd.Add([]string{ls, arg3})
								}
								cmd.Add([]string{echo, arg2})

								subTestName := fmt.Sprintf(
									"ContinueOnErrorIs%tAndIgnoreErrorIs%tAndIncludeBadCmdIs%t",
									continueOnError,
									ignoreError,
									includeBadCmd,
								)
								t.Run(subTestName, func(t *testing.T) {
									if runFuncType == "Parallel" && !continueOnError {
										t.Skip("Continue on error only applies to non parallel executions")
									}
									cmd.ContinueOnError(continueOnError).IgnoreError(ignoreError)
									successful := ignoreError || !includeBadCmd
									outputAfterLsExists := !includeBadCmd || continueOnError
									output := verifyCommandAndGetOutput(ctx, t, cmd, runFunc, successful)
									checkOutput(t, true, output, arg1)
									checkOutput(t, includeBadCmd, output, lsErrorMsg)
									checkOutput(t, outputAfterLsExists, output, arg2)
								})
							}
						},
						"CommandOutputAndErrorIsReadable": func(ctx context.Context, t *testing.T, cmd Command) {
							for subName, subTestCase := range map[string]func(context.Context, *testing.T, Command, cmdRunFunc){
								"StdOutOnly": func(ctx context.Context, t *testing.T, cmd Command, run cmdRunFunc) {
									cmd.Add([]string{echo, arg1})
									cmd.Add([]string{echo, arg2})
									output := verifyCommandAndGetOutput(ctx, t, &cmd, run, true)
									checkOutput(t, true, output, arg1, arg2)
								},
								"StdErrOnly": func(ctx context.Context, t *testing.T, cmd Command, run cmdRunFunc) {
									cmd.Add([]string{ls, arg3})
									output := verifyCommandAndGetOutput(ctx, t, &cmd, run, false)
									checkOutput(t, true, output, lsErrorMsg)
								},
								"StdOutAndStdErr": func(ctx context.Context, t *testing.T, cmd Command, run cmdRunFunc) {
									cmd.Add([]string{echo, arg1})
									cmd.Add([]string{echo, arg2})
									cmd.Add([]string{ls, arg3})
									output := verifyCommandAndGetOutput(ctx, t, &cmd, run, false)
									checkOutput(t, true, output, arg1, arg2, lsErrorMsg)
								},
							} {
								t.Run(subName, func(t *testing.T) {
									subTestCase(ctx, t, cmd, runFunc)
								})
							}
						},
						"WriterOutputAndErrorIsSettable": func(ctx context.Context, t *testing.T, cmd Command) {
							for subName, subTestCase := range map[string]func(context.Context, *testing.T, Command, *Buffer){
								"StdOutOnly": func(ctx context.Context, t *testing.T, cmd Command, buf *Buffer) {
									cmd.SetOutputWriter(buf)
									require.NoError(t, runFunc(&cmd, ctx))
									checkOutput(t, true, buf.String(), arg1, arg2)
									checkOutput(t, false, buf.String(), lsErrorMsg)
								},
								"StdErrOnly": func(ctx context.Context, t *testing.T, cmd Command, buf *Buffer) {
									cmd.SetErrorWriter(buf)
									require.NoError(t, runFunc(&cmd, ctx))
									checkOutput(t, true, buf.String(), lsErrorMsg)
									checkOutput(t, false, buf.String(), arg1, arg2)
								},
								"StdOutAndStdErr": func(ctx context.Context, t *testing.T, cmd Command, buf *Buffer) {
									cmd.SetCombinedWriter(buf)
									require.NoError(t, runFunc(&cmd, ctx))
									checkOutput(t, true, buf.String(), arg1, arg2, lsErrorMsg)
								},
							} {
								t.Run(subName, func(t *testing.T) {
									cmd.Extend([][]string{
										[]string{echo, arg1},
										[]string{echo, arg2},
										[]string{ls, arg3},
									}).ContinueOnError(true).IgnoreError(true)

									var buf bytes.Buffer
									bufCloser := &Buffer{b: buf}

									subTestCase(ctx, t, cmd, bufCloser)
								})
							}
						},
						"SenderOutputAndErrorIsSettable": func(ctx context.Context, t *testing.T, cmd Command) {
							for subName, subTestCase := range map[string]func(context.Context, *testing.T, Command, *send.InMemorySender){
								"StdOutOnly": func(ctx context.Context, t *testing.T, cmd Command, sender *send.InMemorySender) {
									cmd.SetOutputSender(cmd.priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", arg1, arg2)
									checkOutput(t, false, strings.Join(out, "\n"), lsErrorMsg)
								},
								"StdErrOnly": func(ctx context.Context, t *testing.T, cmd Command, sender *send.InMemorySender) {
									cmd.SetErrorSender(cmd.priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", lsErrorMsg)
									checkOutput(t, false, strings.Join(out, "\n"), arg1, arg2)
								},
								"StdOutAndStdErr": func(ctx context.Context, t *testing.T, cmd Command, sender *send.InMemorySender) {
									cmd.SetCombinedSender(cmd.priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", arg1, arg2, lsErrorMsg)
								},
							} {
								t.Run(subName, func(t *testing.T) {
									cmd.Extend([][]string{
										[]string{echo, arg1},
										[]string{echo, arg2},
										[]string{ls, arg3},
									}).ContinueOnError(true).IgnoreError(true).Priority(level.Info)

									levelInfo := send.LevelInfo{Default: cmd.priority, Threshold: cmd.priority}
									sender, err := send.NewInMemorySender(t.Name(), levelInfo, 100)
									require.NoError(t, err)

									subTestCase(ctx, t, cmd, sender.(*send.InMemorySender))
								})
							}
						},
						"GetProcIDsReturnsCorrectNumberOfIDs": func(ctx context.Context, t *testing.T, cmd Command) {
							subCmds := [][]string{
								[]string{echo, arg1},
								[]string{echo, arg2},
								[]string{ls, arg3},
							}
							cmd.Extend(subCmds).ContinueOnError(true).IgnoreError(true).Run(ctx)
							assert.Len(t, cmd.GetProcIDs(), len(subCmds))
						},
						"ApplyFromOptsUpdatesCmdCorrectly": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := &CreateOptions{
								WorkingDirectory: cwd,
							}
							cmd.ApplyFromOpts(opts).Add([]string{ls, cwd})
							output := verifyCommandAndGetOutput(ctx, t, &cmd, runFunc, true)
							checkOutput(t, true, output, "makefile")
						},
						"SetOutputOptions": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := OutputOptions{
								SendOutputToError: true,
							}

							assert.False(t, cmd.opts.Output.SendOutputToError)
							cmd.SetOutputOptions(opts)
							assert.True(t, cmd.opts.Output.SendOutputToError)
						},
						"ApplyFromOptsOverridesExistingOptions": func(ctx context.Context, t *testing.T, cmd Command) {
							_ = cmd.Add([]string{echo, arg1}).Directory("bar")
							genOpts, err := cmd.getCreateOpts(ctx)
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, "bar", genOpts[0].WorkingDirectory)

							opts := &CreateOptions{WorkingDirectory: "foo"}
							_ = cmd.ApplyFromOpts(opts)
							genOpts, err = cmd.getCreateOpts(ctx)
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, opts.WorkingDirectory, genOpts[0].WorkingDirectory)
						},
						"CreateOptionsAppliedInGetCreateOptionsForLocalCommand": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := &CreateOptions{
								WorkingDirectory: "foo",
								Environment:      map[string]string{"foo": "bar"},
							}
							args := []string{echo, arg1}
							cmd.cmds = [][]string{}
							_ = cmd.ApplyFromOpts(opts).Add(args)
							genOpts, err := cmd.getCreateOpts(ctx)
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, opts.WorkingDirectory, genOpts[0].WorkingDirectory)
							assert.Equal(t, opts.Environment, genOpts[0].Environment)
						},
						"DirectoryIsSetInRemoteCommand": func(ctx context.Context, t *testing.T, cmd Command) {
							args := []string{echo, arg1}
							dir := "foo"
							_ = cmd.Host("localhost").Directory(dir).Add(args)
							genOpts, err := cmd.getCreateOpts(ctx)
							require.NoError(t, err)
							require.Len(t, genOpts, 1)

							// The remote command should run with the working
							// directory set, not the local command.
							assert.NotEqual(t, dir, genOpts[0].WorkingDirectory)
							setsDir := false
							for _, args := range genOpts[0].Args {
								if strings.Contains(args, dir) {
									setsDir = true
									break
								}
							}
							assert.True(t, setsDir)
						},
						"EnvironmentIsSetInRemoteCommand": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := &CreateOptions{
								Environment: map[string]string{"foo": "bar"},
							}
							args := []string{echo, arg1}
							_ = cmd.Host("localhost").ApplyFromOpts(opts).Add(args)
							genOpts, err := cmd.getCreateOpts(ctx)
							require.NoError(t, err)
							require.Len(t, genOpts, 1)

							// The remote command should run with the environment
							// set, not the local command.
							assert.Empty(t, genOpts[0].Environment)
							setsEnv := false
							envSlice := opts.getEnvSlice()
							for _, args := range genOpts[0].Args {
								for _, envVarAndValue := range envSlice {
									if !strings.Contains(args, envVarAndValue) {
										continue
									}
								}
								setsEnv = true
								break
							}
							assert.True(t, setsEnv)
						},
						"GetJobs": func(ctx context.Context, t *testing.T, cmd Command) {
							jobs, err := cmd.Append("ls", "echo hi", "ls -lha").Jobs(ctx)
							assert.NoError(t, err)
							assert.Len(t, jobs, 3)
						},
						"GetJobsForeground": func(ctx context.Context, t *testing.T, cmd Command) {
							jobs, err := cmd.Append("ls", "echo hi", "ls -lha").JobsForeground(ctx)
							assert.NoError(t, err)
							assert.Len(t, jobs, 3)
						},
						// "": func(ctx context.Context, t *testing.T, cmd Command) {},
					} {
						t.Run(name, func(t *testing.T) {
							ctx, cancel := context.WithTimeout(context.Background(), taskTimeout)
							defer cancel()

							cmd := NewCommand().ProcConstructor(makep)
							testCase(ctx, t, *cmd)
						})
					}
				})
			}
		})
	}
}

func TestRunParallelRunsInParallel(t *testing.T) {
	cmd := NewCommand().Extend([][]string{
		[]string{"sleep", "3"},
		[]string{"sleep", "3"},
		[]string{"sleep", "3"},
	})
	threePointFiveSeconds := time.Second*3 + time.Millisecond*500
	maxRunTimeAllowed := threePointFiveSeconds
	cctx, cancel := context.WithTimeout(context.Background(), maxRunTimeAllowed)
	defer cancel()
	// If this does not run in parallel, the context will timeout and we will
	// get an error.
	assert.NoError(t, cmd.RunParallel(cctx))
}
