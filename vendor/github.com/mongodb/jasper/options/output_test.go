package options

import (
	"bytes"
	"io/ioutil"
	"strings"
	"testing"

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
		"SuppressOutputWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogDefault,
						Format: RawLoggerConfigFormatBSON,
					},
				},
			}
			opts.SuppressOutput = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogDefault,
						Format: RawLoggerConfigFormatBSON,
					},
				},
			}
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"SuppressOutputAndErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogDefault,
						Format: RawLoggerConfigFormatBSON,
					},
				},
			}
			opts.SuppressOutput = true
			opts.SuppressError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectOutputWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogDefault,
						Format: RawLoggerConfigFormatBSON,
					},
				},
			}
			opts.SendOutputToError = true
			assert.NoError(t, opts.Validate())
		},
		"RedirectErrorWithLogger": func(t *testing.T, opts Output) {
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogDefault,
						Format: RawLoggerConfigFormatBSON,
					},
				},
			}
			opts.SendErrorToOutput = true
			assert.NoError(t, opts.Validate())
		},
		"GetOutputWithStdoutAndLogger": func(t *testing.T, opts Output) {
			opts.Output = stdout
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogInMemory,
						Format: RawLoggerConfigFormatBSON,
					},
					producer: &InMemoryLoggerOptions{
						InMemoryCap: 100,
						Base:        BaseOptions{Format: LogFormatPlain},
					},
				},
			}
			out, err := opts.GetOutput()
			require.NoError(t, err)

			msg := "foo"
			_, err = out.Write([]byte(msg))
			assert.NoError(t, err)
			assert.NoError(t, opts.outputSender.Close())

			assert.Equal(t, msg, stdout.String())

			safeSender, ok := opts.Loggers[0].sender.(*SafeSender)
			require.True(t, ok)
			sender, ok := safeSender.Sender.(*send.InMemorySender)
			require.True(t, ok)

			logOut, err := sender.GetString()
			require.NoError(t, err)
			require.Equal(t, 1, len(logOut))
			assert.Equal(t, msg, strings.Join(logOut, ""))
		},
		"GetErrorWithErrorAndLogger": func(t *testing.T, opts Output) {
			opts.Error = stderr
			opts.Loggers = []*LoggerConfig{
				{
					info: loggerConfigInfo{
						Type:   LogInMemory,
						Format: RawLoggerConfigFormatJSON,
					},
					producer: &InMemoryLoggerOptions{
						InMemoryCap: 100,
						Base:        BaseOptions{Format: LogFormatPlain},
					},
				},
			}
			errOut, err := opts.GetError()
			require.NoError(t, err)

			msg := "foo"
			_, err = errOut.Write([]byte(msg))
			assert.NoError(t, err)
			assert.NoError(t, opts.errorSender.Close())

			assert.Equal(t, msg, stderr.String())

			safeSender, ok := opts.Loggers[0].sender.(*SafeSender)
			require.True(t, ok)
			sender, ok := safeSender.Sender.(*send.InMemorySender)
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
