package options

import (
	"bytes"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputOptions(t *testing.T) {
	stdout := bytes.NewBuffer([]byte{})
	stderr := bytes.NewBuffer([]byte{})

	type testCase func(*testing.T, Output)

	cases := map[string]testCase{
		"NilOptionsValidate": func(t *testing.T, opts Output) {
			assert.Zero(t, opts)
			assert.NoError(t, opts.Validate())
		},
		"ErrorOutputSpecified": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.Error = stderr
			assert.NoError(t, opts.Validate())
		},
		"SuppressErrorWhenSpecified": func(t *testing.T, opts Output) {
			opts.Error = stderr
			opts.SuppressError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressOutputWhenSpecified": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.SuppressOutput = true
			assert.Error(t, opts.Validate())
		},
		"RedirectErrorToNillFails": func(t *testing.T, opts Output) {
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"RedirectOutputToError": func(t *testing.T, opts Output) {
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressAndRedirectOutputIsInvalid": func(t *testing.T, opts Output) {
			opts.SuppressOutput = true
			opts.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"SuppressAndRedirectErrorIsInvalid": func(t *testing.T, opts Output) {
			opts.SuppressError = true
			opts.SendErrorToOutput = true
			assert.Error(t, opts.Validate())
		},
		"DiscardIsNilForOutput": func(t *testing.T, opts Output) {
			opts.Error = stderr
			opts.Output = ioutil.Discard

			assert.True(t, opts.outputIsNull())
			assert.False(t, opts.errorIsNull())
		},
		"NilForOutputIsValid": func(t *testing.T, opts Output) {
			opts.Error = stderr
			assert.True(t, opts.outputIsNull())
			assert.False(t, opts.errorIsNull())
		},
		"DiscardIsNilForError": func(t *testing.T, opts Output) {
			opts.Error = ioutil.Discard
			opts.Output = stdout
			assert.True(t, opts.errorIsNull())
			assert.False(t, opts.outputIsNull())
		},
		"NilForErrorIsValid": func(t *testing.T, opts Output) {
			opts.Output = stdout
			assert.True(t, opts.errorIsNull())
			assert.False(t, opts.outputIsNull())
		},
		"OutputGetterNilIsIoDiscard": func(t *testing.T, opts Output) {
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, ioutil.Discard, out)
		},
		"OutputGetterWhenPopulatedIsCorrect": func(t *testing.T, opts Output) {
			opts.Output = stdout
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, stdout, out)
		},
		"ErrorGetterNilIsIoDiscard": func(t *testing.T, opts Output) {
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, ioutil.Discard, outErr)
		},
		"ErrorGetterWhenPopulatedIsCorrect": func(t *testing.T, opts Output) {
			opts.Error = stderr
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, stderr, outErr)
		},
		"RedirectErrorHasCorrectSemantics": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendErrorToOutput = true
			outErr, err := opts.GetError()
			assert.NoError(t, err)
			assert.Equal(t, stdout, outErr)
		},
		"RedirectOutputHasCorrectSemantics": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendOutputToError = true
			out, err := opts.GetOutput()
			assert.NoError(t, err)
			assert.Equal(t, stderr, out)
		},
		"RedirectCannotHaveCycle": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.Error = stderr
			opts.SendOutputToError = true
			opts.SendErrorToOutput = true
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsForInvalidLogFormat": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormat("foo")}}}
			assert.Error(t, opts.Validate())
		},
		"ValidateFailsForInvalidLogTypes": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogType(""), Options: Log{Format: LogFormatPlain}}}
			assert.Error(t, opts.Validate())
		},
		"SuppressOutputWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormatPlain}}}
			opts.SuppressOutput = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormatPlain}}}
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressOutputAndErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormatPlain}}}
			opts.SuppressOutput = true
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectOutputWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormatPlain}}}
			opts.SendOutputToError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []Logger{{Type: LogDefault, Options: Log{Format: LogFormatPlain}}}
			opts.SendErrorToOutput = true
			assert.NoError(t, opts.Validate())
		},
		"GetOutputWithStdoutAndLogger": func(t *testing.T, opts Output) {
			opts.Output = stdout
			logger := Logger{Type: LogInMemory, Options: Log{Format: LogFormatPlain, InMemoryCap: 100}}
			opts.Loggers = []Logger{logger}
			out, err := opts.GetOutput()
			require.NoError(t, err)

			msg := "foo"
			_, err = out.Write([]byte(msg))
			assert.NoError(t, err)
			assert.NoError(t, opts.outputSender.Close())

			assert.Equal(t, msg, stdout.String())

			sender, ok := opts.Loggers[0].sender.(*send.InMemorySender)
			require.True(t, ok)

			logOut, err := sender.GetString()
			require.NoError(t, err)
			require.Equal(t, 1, len(logOut))
			assert.Equal(t, msg, strings.Join(logOut, ""))
		},
		"GetErrorWithErrorAndLogger": func(t *testing.T, opts Output) {
			opts.Error = stderr
			logger := Logger{Type: LogInMemory, Options: Log{Format: LogFormatPlain, InMemoryCap: 100}}
			opts.Loggers = []Logger{logger}
			errOut, err := opts.GetError()
			require.NoError(t, err)

			msg := "foo"
			_, err = errOut.Write([]byte(msg))
			assert.NoError(t, err)
			assert.NoError(t, opts.errorSender.Close())

			assert.Equal(t, msg, stderr.String())

			sender, ok := opts.Loggers[0].sender.(*send.InMemorySender)
			require.True(t, ok)

			logErr, err := sender.GetString()
			require.NoError(t, err)
			require.Equal(t, 1, len(logErr))
			assert.Equal(t, msg, strings.Join(logErr, ""))
		},
		// "": func(t *testing.T, opts Output) {}
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			test(t, Output{})
		})
	}
}

func TestOutputIntegrationTableTest(t *testing.T) {
	buf := &bytes.Buffer{}
	shouldFail := []Output{
		{Output: buf, SendOutputToError: true},
	}

	shouldPass := []Output{
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
			file, err := ioutil.TempFile("", "foo.txt")
			require.NoError(t, err)
			defer func() {
				assert.NoError(t, file.Close())
				grip.Warning(os.RemoveAll(file.Name()))
			}()

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
		// "": func(t *testing.T, l LogType, opts Log) {},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			test(t, Logger{Type: LogDefault, Options: Log{Format: LogFormatPlain}})
		})
	}
}
