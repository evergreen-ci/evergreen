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
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
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
		assert.True(t, exists == strings.Contains(output, expected), "out='%s' and expected='%s'", output, expected)
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
						"WaitErrorsIfNeverStarted": func(ctx context.Context, t *testing.T, cmd Command) {
							_, err := cmd.Wait(ctx)
							assert.Error(t, err)
						},
						"WaitErrorsIfProcessErrors": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.Append("false")
							assert.Error(t, runFunc(&cmd, ctx))
							exitCode, err := cmd.Wait(ctx)
							assert.Error(t, err)
							assert.NotZero(t, exitCode)
						},
						"WaitOnBackgroundRunWaitsForProcessCompletion": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.Append("sleep 1", "sleep 1").Background(true)
							require.NoError(t, runFunc(&cmd, ctx))
							exitCode, err := cmd.Wait(ctx)
							assert.NoError(t, err)
							assert.Zero(t, exitCode)

							for _, proc := range cmd.procs {
								assert.True(t, proc.Info(ctx).Complete)
								assert.True(t, proc.Info(ctx).Successful)
							}
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

											allOpts, err := cmd.getCreateOpts()
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

											allOpts, err := cmd.getCreateOpts()
											require.NoError(t, err)
											require.Len(t, allOpts, 1)
											checkArgs(allOpts[0].Args, cmd1)

											cmd.Append(cmd2)
											allOpts, err = cmd.getCreateOpts()
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

											allOpts, err := cmd.getCreateOpts()
											require.NoError(t, err)
											require.Len(t, allOpts, 1)
											checkArgs(allOpts[0].Args, cmd1)

											cmd.Add([]string{echo, arg2})
											allOpts, err = cmd.getCreateOpts()
											require.NoError(t, err)
											require.Len(t, allOpts, 2)
											checkArgs(allOpts[0].Args, cmd1)
											checkArgs(allOpts[1].Args, cmd2)
										},
									} {
										t.Run(subTestName, func(t *testing.T) {
											cmd = *NewCommand().ProcConstructor(makep)
											if isRemote {
												cmd.User(user).Host("localhost").Password("password")
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
									{echo, arg1},
									{ls, arg2},
									{echo, arg3},
								},
							).Directory(cwd)
							assert.Error(t, runFunc(&cmd, ctx))
						},
						"ExecutionFlags": func(ctx context.Context, t *testing.T, cmd Command) {
							numCombinations := int(math.Pow(2, 3))
							for i := 0; i < numCombinations; i++ {
								continueOnError, ignoreError, includeBadCmd := i&1 == 1, i&2 == 2, i&4 == 4

								cmd := NewCommand().Add([]string{echo, arg1}).ProcConstructor(makep)
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
									cmd = *NewCommand().ProcConstructor(makep)
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
									cmd = *NewCommand().ProcConstructor(makep).Extend([][]string{
										{echo, arg1},
										{echo, arg2},
										{ls, arg3},
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
									cmd.SetOutputSender(cmd.opts.Priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", arg1, arg2)
									checkOutput(t, false, strings.Join(out, "\n"), lsErrorMsg)
								},
								"StdErrOnly": func(ctx context.Context, t *testing.T, cmd Command, sender *send.InMemorySender) {
									cmd.SetErrorSender(cmd.opts.Priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", lsErrorMsg)
									checkOutput(t, false, strings.Join(out, "\n"), arg1, arg2)
								},
								"StdOutAndStdErr": func(ctx context.Context, t *testing.T, cmd Command, sender *send.InMemorySender) {
									cmd.SetCombinedSender(cmd.opts.Priority, sender)
									require.NoError(t, runFunc(&cmd, ctx))
									out, err := sender.GetString()
									require.NoError(t, err)
									checkOutput(t, true, strings.Join(out, "\n"), "[p=info]:", arg1, arg2, lsErrorMsg)
								},
							} {
								t.Run(subName, func(t *testing.T) {
									cmd = *NewCommand().ProcConstructor(makep).Extend([][]string{
										{echo, arg1},
										{echo, arg2},
										{ls, arg3},
									}).ContinueOnError(true).IgnoreError(true).Priority(level.Info)

									levelInfo := send.LevelInfo{Default: cmd.opts.Priority, Threshold: cmd.opts.Priority}
									sender, err := send.NewInMemorySender(t.Name(), levelInfo, 100)
									require.NoError(t, err)

									subTestCase(ctx, t, cmd, sender.(*send.InMemorySender))
								})
							}
						},
						"GetProcIDsReturnsCorrectNumberOfIDs": func(ctx context.Context, t *testing.T, cmd Command) {
							subCmds := [][]string{
								{echo, arg1},
								{echo, arg2},
								{ls, arg3},
							}
							assert.NoError(t, cmd.Extend(subCmds).ContinueOnError(true).IgnoreError(true).Run(ctx))
							assert.Len(t, cmd.GetProcIDs(), len(subCmds))
						},
						"ApplyFromOptsUpdatesCmdCorrectly": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := &options.Create{
								WorkingDirectory: cwd,
							}
							cmd.ApplyFromOpts(opts).Add([]string{ls, cwd})
							output := verifyCommandAndGetOutput(ctx, t, &cmd, runFunc, true)
							checkOutput(t, true, output, "makefile")
						},
						"SetOutputOptions": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := options.Output{
								SendOutputToError: true,
							}

							assert.False(t, cmd.opts.Process.Output.SendOutputToError)
							cmd.SetOutputOptions(opts)
							assert.True(t, cmd.opts.Process.Output.SendOutputToError)
						},
						"ApplyFromOptsOverridesExistingOptions": func(ctx context.Context, t *testing.T, cmd Command) {
							_ = cmd.Add([]string{echo, arg1}).Directory("bar")
							genOpts, err := cmd.getCreateOpts()
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, "bar", genOpts[0].WorkingDirectory)

							opts := &options.Create{WorkingDirectory: "foo"}
							_ = cmd.ApplyFromOpts(opts)
							genOpts, err = cmd.getCreateOpts()
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, opts.WorkingDirectory, genOpts[0].WorkingDirectory)
						},
						"CreateOptionsAppliedInGetCreateOptionsForLocalCommand": func(ctx context.Context, t *testing.T, cmd Command) {
							opts := &options.Create{
								WorkingDirectory: "foo",
								Environment:      map[string]string{"foo": "bar"},
							}
							args := []string{echo, arg1}
							cmd.opts.Commands = [][]string{}
							_ = cmd.ApplyFromOpts(opts).Add(args)
							genOpts, err := cmd.getCreateOpts()
							require.NoError(t, err)
							require.Len(t, genOpts, 1)
							assert.Equal(t, opts.WorkingDirectory, genOpts[0].WorkingDirectory)
							assert.Equal(t, opts.Environment, genOpts[0].Environment)
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
						"TagFunctions": func(ctx context.Context, t *testing.T, cmd Command) {
							tags := []string{"tag0", "tag1"}
							subCmds := []string{"echo hi", "echo bye"}
							for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, cmd Command){
								"SetTags": func(ctx context.Context, t *testing.T, cmd Command) {
									for _, subCmd := range subCmds {
										cmd.Append(subCmd)
									}
									cmd.SetTags(tags)
									require.NoError(t, cmd.Run(ctx))
									assert.Len(t, cmd.procs, len(subCmds))
									for _, proc := range cmd.procs {
										assert.Subset(t, tags, proc.GetTags())
										assert.Subset(t, proc.GetTags(), tags)
									}
								},
								"AppendTags": func(ctx context.Context, t *testing.T, cmd Command) {
									for _, subCmd := range subCmds {
										cmd.Append(subCmd)
									}
									cmd.AppendTags(tags...)
									require.NoError(t, cmd.Run(ctx))
									assert.Len(t, cmd.procs, len(subCmds))
									for _, proc := range cmd.procs {
										assert.Subset(t, tags, proc.GetTags())
										assert.Subset(t, proc.GetTags(), tags)
									}
								},
								"ExtendTags": func(ctx context.Context, t *testing.T, cmd Command) {
									for _, subCmd := range subCmds {
										cmd.Append(subCmd)
									}
									cmd.ExtendTags(tags)
									require.NoError(t, cmd.Run(ctx))
									assert.Len(t, cmd.procs, len(subCmds))
									for _, proc := range cmd.procs {
										assert.Subset(t, tags, proc.GetTags())
										assert.Subset(t, proc.GetTags(), tags)
									}
								},
							} {
								t.Run(subTestName, func(t *testing.T) {
									cmd = *NewCommand().ProcConstructor(makep)
									subTestCase(ctx, t, cmd)
								})
							}
						},
						"SingleArgCommandSplitsShellCommandCorrectly": func(ctx context.Context, t *testing.T, cmd Command) {
							cmd.Extend([][]string{
								{"echo hello world"},
								{"echo 'hello world'"},
								{"echo 'hello\"world\"'"},
							})

							optslist, err := cmd.Export()
							require.NoError(t, err)

							require.Len(t, optslist, 3)
							assert.Equal(t, []string{"echo", "hello", "world"}, optslist[0].Args)
							assert.Equal(t, []string{"echo", "hello world"}, optslist[1].Args)
							assert.Equal(t, []string{"echo", "hello\"world\""}, optslist[2].Args)
						},
						"RunFuncReceivesPopulatedOptions": func(ctx context.Context, t *testing.T, cmd Command) {
							prio := level.Warning
							user := "user"
							runFuncCalled := false
							cmd.Add([]string{echo, arg1}).
								ContinueOnError(true).IgnoreError(true).
								Priority(prio).Background(true).
								Sudo(true).SudoAs(user).
								SetRunFunc(func(opts options.Command) error {
									runFuncCalled = true
									assert.True(t, opts.ContinueOnError)
									assert.True(t, opts.IgnoreError)
									assert.True(t, opts.RunBackground)
									assert.True(t, opts.Sudo)
									assert.Equal(t, user, opts.SudoUser)
									assert.Equal(t, prio, opts.Priority)
									return nil
								})
							require.NoError(t, cmd.Run(ctx))
							assert.True(t, runFuncCalled)
						},
						// "": func(ctx context.Context, t *testing.T, cmd Command) {},
					} {
						t.Run(name, func(t *testing.T) {
							ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
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
		{"sleep", "3"},
		{"sleep", "3"},
		{"sleep", "3"},
	})
	threePointFiveSeconds := time.Second*3 + time.Millisecond*500
	maxRunTimeAllowed := threePointFiveSeconds
	cctx, cancel := context.WithTimeout(context.Background(), maxRunTimeAllowed)
	defer cancel()
	// If this does not run in parallel, the context will timeout and we will
	// get an error.
	assert.NoError(t, cmd.RunParallel(cctx))
}
