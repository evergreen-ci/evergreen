package jasper

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputOptions(t *testing.T) {
	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	type testCase func(*testing.T, OutputOptions)

	cases := map[string]testCase{
		"NilOptionsValidate": func(t *testing.T, opts OutputOptions) {
			assert.Zero(t, opts)
			assert.NoError(t, opts.Validate())
		},
		"ErrorOutputSpecified": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			opts.Error = stderr
			assert.NoError(t, opts.Validate())
		},
		"SuppressErrorWhenSpecified": func(t *testing.T, opts OutputOptions) {
			opts.Error = stderr
			opts.SuppressError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressOutputWhenSpecified": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			opts.SuppressOutput = true
			assert.Error(t, opts.Validate())
		},
		"RedirectErrorToNillFails": func(t *testing.T, opts OutputOptions) {
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"RedirectOutputToError": func(t *testing.T, opts OutputOptions) {
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressAndRedirectOutputIsInvalid": func(t *testing.T, opts OutputOptions) {
			opts.SuppressOutput = true
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressAndRedirectErrorIsInvalid": func(t *testing.T, opts OutputOptions) {
			opts.SuppressError = true
			opts.SendErrorToOutput = true
			assert.Error(t, opts.Validate())
		},
		"DiscardIsNilForOutput": func(t *testing.T, opts OutputOptions) {
			opts.Error = stderr
			opts.Output = ioutil.Discard

			assert.True(t, opts.outputIsNull())
			assert.False(t, opts.errorIsNull())
		},
		"NilForOutputIsValid": func(t *testing.T, opts OutputOptions) {
			opts.Error = stderr
			assert.True(t, opts.outputIsNull())
			assert.False(t, opts.errorIsNull())
		},
		"DiscardIsNilForError": func(t *testing.T, opts OutputOptions) {
			opts.Error = ioutil.Discard
			opts.Output = stdout
			assert.True(t, opts.errorIsNull())
			assert.False(t, opts.outputIsNull())
		},
		"NilForErrorIsValid": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			assert.True(t, opts.errorIsNull())
			assert.False(t, opts.outputIsNull())
		},
		"OutputGetterNilIsIoDiscard": func(t *testing.T, opts OutputOptions) {
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, ioutil.Discard, out)
		},
		"OutputGetterWhenPopulatedIsCorrect": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, stdout, out)
		},
		"ErrorGetterNilIsIoDiscard": func(t *testing.T, opts OutputOptions) {
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, ioutil.Discard, outErr)
		},
		"ErrorGetterWhenPopulatedIsCorrect": func(t *testing.T, opts OutputOptions) {
			opts.Error = stderr
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, stderr, outErr)
		},
		"RedirectErrorHasCorrectSemantics": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendErrorToOutput = true
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, stdout, outErr)
		},
		"RedirectOutputHasCorrectSemantics": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendOutputToError = true
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, stderr, out)
		},
		"RedirectCannotHaveCycle": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendOutputToError = true
			opts.SendErrorToOutput = true
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsForInvalidLogFormat": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormat("foo")}}}
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsForInvalidLogTypes": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogType(""), Options: LogOptions{Format: LogFormatPlain}}}
			assert.Error(t, opts.Validate())
		},
		"SuppressOutputWithLogger": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
			opts.SuppressOutput = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressErrorWithLogger": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressOutputAndErrorWithLogger": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
			opts.SuppressOutput = true
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectOutputWithLogger": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
			opts.SendOutputToError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectErrorWithLogger": func(t *testing.T, opts OutputOptions) {
			opts.Loggers = []Logger{Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}}}
			opts.SendErrorToOutput = true
			assert.NoError(t, opts.Validate())
		},
		"GetOutputWithStdoutAndLogger": func(t *testing.T, opts OutputOptions) {
			opts.Output = stdout
			logger := Logger{Type: LogInMemory, Options: LogOptions{Format: LogFormatPlain, InMemoryCap: 100}}
			opts.Loggers = []Logger{logger}
			out, err := opts.GetOutput()
			require.NoError(t, err)

			msg := "foo"
			out.Write([]byte(msg))
			opts.outputSender.Close()

			assert.Equal(t, msg, stdout.String())

			sender, ok := opts.Loggers[0].sender.(*send.InMemorySender)
			require.True(t, ok)

			logOut, err := sender.GetString()
			require.NoError(t, err)
			require.Equal(t, 1, len(logOut))
			assert.Equal(t, msg, strings.Join(logOut, ""))
		},
		"GetErrorWithErrorAndLogger": func(t *testing.T, opts OutputOptions) {
			opts.Error = stderr
			logger := Logger{Type: LogInMemory, Options: LogOptions{Format: LogFormatPlain, InMemoryCap: 100}}
			opts.Loggers = []Logger{logger}
			errOut, err := opts.GetError()
			require.NoError(t, err)

			msg := "foo"
			errOut.Write([]byte(msg))
			opts.errorSender.Close()

			assert.Equal(t, msg, stderr.String())

			sender, ok := opts.Loggers[0].sender.(*send.InMemorySender)
			require.True(t, ok)

			logErr, err := sender.GetString()
			require.NoError(t, err)
			require.Equal(t, 1, len(logErr))
			assert.Equal(t, msg, strings.Join(logErr, ""))
		},
		// "": func(t *testing.T, opts OutputOptions) {}
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			test(t, OutputOptions{})
		})
	}
}

func TestOutputOptionsIntegrationTableTest(t *testing.T) {
	buf := &bytes.Buffer{}
	shouldFail := []OutputOptions{
		{Output: buf, SendOutputToError: true},
	}

	shouldPass := []OutputOptions{
		{Output: buf, Error: buf},
		{SuppressError: true, SuppressOutput: true},
		{Output: buf, SendErrorToOutput: true},
	}

	for idx, opt := range shouldFail {
		assert.Error(t, opt.Validate(), "%d: %+v", idx, opt)
	}

	for idx, opt := range shouldPass {
		assert.NoError(t, opt.Validate(), "%d: %+v", idx, opt)
	}

}

func TestLoggers(t *testing.T) {
	type testCase func(*testing.T, Logger)
	cases := map[string]testCase{
		"InvalidLogTypeFails": func(t *testing.T, l Logger) {
			l.Type = LogType("")
			assert.Error(t, l.Validate())
		},
		"ValidLogTypePasses": func(t *testing.T, l Logger) {
			assert.NoError(t, l.Validate())
		},
		"ConfigureFailsForInvalidLogType": func(t *testing.T, l Logger) {
			l.Type = LogType("foo")
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigurePassesWithLogDefault": func(t *testing.T, l Logger) {
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigurePassesWithLogInherit": func(t *testing.T, l Logger) {
			l.Type = LogInherit
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigureFailsWithoutPopulatedSplunkOptions": func(t *testing.T, l Logger) {
			l.Type = LogSplunk
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigurePassesWithPopulatedSplunkOptions": func(t *testing.T, l Logger) {
			l.Type = LogSplunk
			l.Options.SplunkOptions = send.SplunkConnectionInfo{ServerURL: "foo", Token: "bar"}
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigureFailsWithoutLocalInBuildloggerOptions": func(t *testing.T, l Logger) {
			l.Type = LogBuildloggerV2
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureFailsWithoutSetNameInBuildloggerOptions": func(t *testing.T, l Logger) {
			l.Type = LogBuildloggerV2
			l.Options.BuildloggerOptions = send.BuildloggerConfig{Local: send.MakeNative()}
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureFailsWithoutPopulatedSumologicOptions": func(t *testing.T, l Logger) {
			l.Type = LogSumologic
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureFailsWithInvalidSumologicOptions": func(t *testing.T, l Logger) {
			l.Type = LogSumologic
			l.Options.SumoEndpoint = "foo"
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureLogFilePasses": func(t *testing.T, l Logger) {
			file, err := ioutil.TempFile("build", "foo.txt")
			require.NoError(t, err)
			defer os.Remove(file.Name())

			l.Type = LogFile
			l.Options.FileName = file.Name()

			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigureFailsWithoutCapacity": func(t *testing.T, l Logger) {
			l.Type = LogInMemory
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigurePassesWithCapacity": func(t *testing.T, l Logger) {
			l.Type = LogInMemory
			l.Options.InMemoryCap = 10
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigurePassesWithBuffering": func(t *testing.T, l Logger) {
			l.Options.BufferOptions.Buffered = true
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigureFailsWithNegativeBufferDuration": func(t *testing.T, l Logger) {
			l.Options.BufferOptions.Buffered = true
			l.Options.BufferOptions.Duration = -1
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureFailsWithNegativeBufferSize": func(t *testing.T, l Logger) {
			l.Options.BufferOptions.Buffered = true
			l.Options.BufferOptions.MaxSize = -1
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigureFailsWithInvalidLogFormat": func(t *testing.T, l Logger) {
			l.Options.Format = LogFormat("foo")
			sender, err := l.Configure()
			assert.Error(t, err)
			assert.Nil(t, sender)
		},
		"ConfigurePassesWithLogFormatDefault": func(t *testing.T, l Logger) {
			l.Options.Format = LogFormatDefault
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigurePassesWithLogFormatJSON": func(t *testing.T, l Logger) {
			l.Options.Format = LogFormatJSON
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		"ConfigurePassesWithLogFormatPlain": func(t *testing.T, l Logger) {
			l.Options.Format = LogFormatPlain
			sender, err := l.Configure()
			assert.NoError(t, err)
			assert.NotNil(t, sender)
		},
		// "": func(t *testing.T, l LogType, opts LogOptions) {},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			test(t, Logger{Type: LogDefault, Options: LogOptions{Format: LogFormatPlain}})
		})
	}
}

func TestGetInMemoryLogStream(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for procType, makeProc := range map[string]ProcessConstructor{
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	} {
		t.Run(procType, func(t *testing.T) {

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string){
				"FailsWithNilProcess": func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string) {
					logs, err := GetInMemoryLogStream(ctx, nil, 1)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"FailsWithInvalidCount": func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string) {
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 0)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"FailsWithoutInMemoryLogger": func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string) {
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.Error(t, err)
					assert.Nil(t, logs)
				},
				"SucceedsWithInMemoryLogger": func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string) {
					opts.Output.Loggers = []Logger{
						{
							Type: LogInMemory,
							Options: LogOptions{
								Format:      LogFormatPlain,
								InMemoryCap: 100,
							},
						},
					}
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.NoError(t, err)
					assert.Contains(t, logs, output)
				},
				"MultipleInMemoryLoggersReturnsLogsFromOnlyOne": func(ctx context.Context, t *testing.T, opts *CreateOptions, makeProc ProcessConstructor, output string) {
					opts.Output.Loggers = []Logger{
						{
							Type: LogInMemory,
							Options: LogOptions{
								Format:      LogFormatPlain,
								InMemoryCap: 100,
							},
						},
						{
							Type: LogInMemory,
							Options: LogOptions{
								Format:      LogFormatPlain,
								InMemoryCap: 100,
							},
						},
					}
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					logs, err := GetInMemoryLogStream(ctx, proc, 100)
					assert.NoError(t, err)
					assert.Contains(t, logs, output)

					outputCount := 0
					for _, log := range logs {
						if strings.Contains(log, output) {
							outputCount++
						}
					}
					assert.Equal(t, 1, outputCount)
				},
				// "SuccessiveCallsReturnLogs": func(ctx context.Context, t *testing.T, opts *CreateOptions, output string) {},
				// "": func(ctx context.Context, t *testing.T, opts *CreateOptions, output string) {},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, tcancel := context.WithTimeout(ctx, processTestTimeout)
					defer tcancel()

					output := "foo"
					opts := &CreateOptions{Args: []string{"echo", output}}
					testCase(tctx, t, opts, makeProc, output)
				})
			}

		})
	}
}
