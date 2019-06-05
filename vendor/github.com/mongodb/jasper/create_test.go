package jasper

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateConstructor(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		id         string
		shouldFail bool
		cmd        string
		args       []string
	}{
		{
			id:         "EmptyString",
			shouldFail: true,
		},
		{
			id:         "BasicCmd",
			args:       []string{"ls", "-lha"},
			cmd:        "ls -lha",
			shouldFail: false,
		},
		{
			id:         "SkipsCommentsAtBeginning",
			shouldFail: true,
			cmd:        "# wat",
		},
		{
			id:         "SkipsCommentsAtEnd",
			cmd:        "ls #what",
			args:       []string{"ls"},
			shouldFail: false,
		},
		{
			id:         "UnbalancedShellLex",
			cmd:        "' foo",
			shouldFail: true,
		},
	} {
		t.Run(test.id, func(t *testing.T) {
			opt, err := MakeCreationOptions(test.cmd)
			if test.shouldFail {
				assert.Error(t, err)
				assert.Nil(t, opt)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, opt)
			assert.Equal(t, test.args, opt.Args)
		})
	}
}

func TestCreateOptions(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for name, test := range map[string]func(t *testing.T, opts *CreateOptions){
		"DefaultConfigForTestsValidate": func(t *testing.T, opts *CreateOptions) {
			assert.NoError(t, opts.Validate())
		},
		"EmptyArgsShouldNotValidate": func(t *testing.T, opts *CreateOptions) {
			opts.Args = []string{}
			assert.Error(t, opts.Validate())
		},
		"ZeroTimeoutShouldNotError": func(t *testing.T, opts *CreateOptions) {
			opts.Timeout = 0
			assert.NoError(t, opts.Validate())
		},
		"SmallTimeoutShouldNotValidate": func(t *testing.T, opts *CreateOptions) {
			opts.Timeout = time.Millisecond
			assert.Error(t, opts.Validate())
		},
		"LargeTimeoutShouldValidate": func(t *testing.T, opts *CreateOptions) {
			opts.Timeout = time.Hour
			assert.NoError(t, opts.Validate())
		},
		"NonExistingWorkingDirectoryShouldNotValidate": func(t *testing.T, opts *CreateOptions) {
			opts.WorkingDirectory = "foo"
			assert.Error(t, opts.Validate())
		},
		"ExtantWorkingDirectoryShouldPass": func(t *testing.T, opts *CreateOptions) {
			wd, err := os.Getwd()
			assert.NoError(t, err)
			assert.NotZero(t, wd)

			opts.WorkingDirectory = wd
			assert.NoError(t, opts.Validate())
		},
		"WorkingDirectoryShouldErrorForFiles": func(t *testing.T, opts *CreateOptions) {
			gobin, err := exec.LookPath("go")
			assert.NoError(t, err)
			assert.NotZero(t, gobin)

			opts.WorkingDirectory = gobin
			assert.Error(t, opts.Validate())
		},
		"MustSpecifyValidOutputOptions": func(t *testing.T, opts *CreateOptions) {
			opts.Output.SendErrorToOutput = true
			opts.Output.SendOutputToError = true
			assert.Error(t, opts.Validate())
		},
		"WorkingDirectoryUnresolveableShouldNotError": func(t *testing.T, opts *CreateOptions) {
			cmd, _, err := opts.Resolve(ctx)
			require.NoError(t, err)
			assert.NotNil(t, cmd)
			assert.NotZero(t, cmd.Dir)
			assert.Equal(t, opts.WorkingDirectory, cmd.Dir)
		},
		"ResolveFailsIfOptionsAreFatal": func(t *testing.T, opts *CreateOptions) {
			opts.Args = []string{}
			cmd, _, err := opts.Resolve(ctx)
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
		"WithoutOverrideEnvironmentEnvIsPopulated": func(t *testing.T, opts *CreateOptions) {
			cmd, _, err := opts.Resolve(ctx)
			assert.NoError(t, err)
			assert.NotZero(t, cmd.Env)
		},
		"WithOverrideEnvironmentEnvIsEmpty": func(t *testing.T, opts *CreateOptions) {
			opts.OverrideEnviron = true
			cmd, _, err := opts.Resolve(ctx)
			assert.NoError(t, err)
			assert.Zero(t, cmd.Env)
		},
		"EnvironmentVariablesArePropagated": func(t *testing.T, opts *CreateOptions) {
			opts.Environment = map[string]string{
				"foo": "bar",
			}

			cmd, _, err := opts.Resolve(ctx)
			assert.NoError(t, err)
			assert.Contains(t, cmd.Env, "foo=bar")
			assert.NotContains(t, cmd.Env, "bar=foo")
		},
		"MultipleArgsArePropagated": func(t *testing.T, opts *CreateOptions) {
			opts.Args = append(opts.Args, "-lha")
			cmd, _, err := opts.Resolve(ctx)
			assert.NoError(t, err)
			assert.Contains(t, cmd.Path, "ls")
			assert.Len(t, cmd.Args, 2)
		},
		"WithOnlyCommandsArgsHasOneVal": func(t *testing.T, opts *CreateOptions) {
			cmd, _, err := opts.Resolve(ctx)
			assert.NoError(t, err)
			assert.Contains(t, cmd.Path, "ls")
			assert.Len(t, cmd.Args, 1)
			assert.Equal(t, "ls", cmd.Args[0])
		},
		"WithTimeout": func(t *testing.T, opts *CreateOptions) {
			opts.Timeout = time.Second
			opts.Args = []string{"sleep", "2"}

			cmd, deadline, err := opts.Resolve(ctx)
			require.NoError(t, err)
			assert.True(t, time.Now().Before(deadline))
			assert.Error(t, cmd.Run())
			assert.True(t, time.Now().After(deadline))
		},
		"ReturnedContextWrapsResolveContext": func(t *testing.T, opts *CreateOptions) {
			opts = sleepCreateOpts(10)
			opts.Timeout = 2 * time.Second
			tctx, tcancel := context.WithTimeout(ctx, time.Millisecond)
			defer tcancel()

			cmd, deadline, err := opts.Resolve(ctx)
			require.NoError(t, err)
			assert.Error(t, cmd.Run())
			assert.Equal(t, context.DeadlineExceeded, tctx.Err())
			assert.True(t, time.Now().After(deadline))
		},
		"ReturnedContextErrorsOnTimeout": func(t *testing.T, opts *CreateOptions) {
			opts = sleepCreateOpts(10)
			opts.Timeout = time.Second
			tctx, tcancel := context.WithTimeout(ctx, 5*time.Second)
			defer tcancel()

			start := time.Now()
			cmd, deadline, err := opts.Resolve(ctx)
			require.NoError(t, err)
			assert.Error(t, cmd.Run())
			elapsed := time.Since(start)
			assert.True(t, elapsed > opts.Timeout)
			assert.NoError(t, tctx.Err())
			assert.True(t, time.Now().After(deadline))
		},
		"ClosersAreAlwaysCalled": func(t *testing.T, opts *CreateOptions) {
			var counter int
			opts.closers = append(opts.closers,
				func() (_ error) { counter++; return },
				func() (_ error) { counter += 2; return },
			)
			assert.NoError(t, opts.Close())
			assert.Equal(t, counter, 3)

		},
		"ConflictingTimeoutOptions": func(t *testing.T, opts *CreateOptions) {
			opts.TimeoutSecs = 100
			opts.Timeout = time.Hour

			assert.Error(t, opts.Validate())
		},
		"ValidationOverrideDefaultsForSecond": func(t *testing.T, opts *CreateOptions) {
			opts.TimeoutSecs = 100
			opts.Timeout = 0

			assert.NoError(t, opts.Validate())
			assert.Equal(t, 100*time.Second, opts.Timeout)
		},
		"ValidationOverrideDefaultsForDuration": func(t *testing.T, opts *CreateOptions) {
			opts.TimeoutSecs = 0
			opts.Timeout = time.Second

			assert.NoError(t, opts.Validate())
			assert.Equal(t, 1, opts.TimeoutSecs)
		},
		"ResolveFailsWithInvalidLoggingConfiguration": func(t *testing.T, opts *CreateOptions) {
			opts.Output.Loggers = []Logger{Logger{Type: LogSumologic, Options: LogOptions{Format: LogFormatPlain}}}
			cmd, _, err := opts.Resolve(ctx)
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
		"ResolveFailsWithInvalidErrorLoggingConfiguration": func(t *testing.T, opts *CreateOptions) {
			opts.Output.Loggers = []Logger{Logger{Type: LogSumologic, Options: LogOptions{Format: LogFormatPlain}}}
			opts.Output.SuppressOutput = true
			cmd, _, err := opts.Resolve(ctx)
			assert.Error(t, err)
			assert.Nil(t, cmd)
		},
	} {
		t.Run(name, func(t *testing.T) {
			opts := &CreateOptions{Args: []string{"ls"}}
			test(t, opts)
		})
	}
}

func TestFileLogging(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	badFileName := "this_does_not_exist"
	// Ensure bad file to cat doesn't exist so that command will write error message to standard error
	_, err := os.Stat(badFileName)
	require.True(t, os.IsNotExist(err))

	catOutputMessage := "foobar"
	outputSize := int64(len(catOutputMessage) + 1)
	catErrorMessage := "cat: this_does_not_exist: No such file or directory"
	errorSize := int64(len(catErrorMessage) + 1)

	// Ensure good file exists and has data
	goodFile, err := ioutil.TempFile("build", "this_file_exists")
	require.NoError(t, err)
	defer os.Remove(goodFile.Name())

	goodFileName := goodFile.Name()
	numBytes, err := goodFile.Write([]byte(catOutputMessage))
	require.NoError(t, err)
	require.NotZero(t, numBytes)

	type Command struct {
		args         []string
		usesGoodFile bool
		usesBadFile  bool
	}
	commands := map[string]Command{
		"Output": Command{
			args:         []string{"cat", goodFileName},
			usesGoodFile: true,
			usesBadFile:  false,
		},
		"Error": Command{
			args:         []string{"cat", badFileName},
			usesGoodFile: false,
			usesBadFile:  true,
		},
		"OutputAndError": Command{
			args:         []string{"cat", goodFileName, badFileName},
			usesGoodFile: true,
			usesBadFile:  true,
		},
	}

	for _, testParams := range []struct {
		id               string
		command          Command
		numBytesExpected int64
		numLogs          int
		outOpts          OutputOptions
	}{
		{
			id:               "LoggerWritesOutputToOneFileEndpoint",
			command:          commands["Output"],
			numBytesExpected: outputSize,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: false},
		},
		{
			id:               "LoggerWritesOutputToMultipleFileEndpoints",
			command:          commands["Output"],
			numBytesExpected: outputSize,
			numLogs:          2,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: false},
		},
		{
			id:               "LoggerWritesErrorToFileEndpoint",
			command:          commands["Error"],
			numBytesExpected: errorSize,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: true, SuppressError: false},
		},
		{
			id:               "LoggerReadsFromBothStandardOutputAndStandardError",
			command:          commands["OutputAndError"],
			numBytesExpected: outputSize + errorSize,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: false},
		},
		{
			id:               "LoggerIgnoresOutputWhenSuppressed",
			command:          commands["Output"],
			numBytesExpected: 0,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: true, SuppressError: false},
		},
		{
			id:               "LoggerIgnoresErrorWhenSuppressed",
			command:          commands["Error"],
			numBytesExpected: 0,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: true},
		},
		{
			id:               "LoggerIgnoresOutputAndErrorWhenSuppressed",
			command:          commands["OutputAndError"],
			numBytesExpected: 0,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: true, SuppressError: true},
		},
		{
			id:               "LoggerReadsFromRedirectedOutput",
			command:          commands["Output"],
			numBytesExpected: outputSize,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: false, SendOutputToError: true},
		},
		{
			id:               "LoggerReadsFromRedirectedError",
			command:          commands["Error"],
			numBytesExpected: errorSize,
			numLogs:          1,
			outOpts:          OutputOptions{SuppressOutput: false, SuppressError: false, SendErrorToOutput: true},
		},
	} {
		t.Run(testParams.id, func(t *testing.T) {

			files := []*os.File{}
			for i := 0; i < testParams.numLogs; i++ {
				file, err := ioutil.TempFile("build", "out.txt")
				require.NoError(t, err)
				defer os.Remove(file.Name())
				info, err := file.Stat()
				require.NoError(t, err)
				assert.Zero(t, info.Size())
				files = append(files, file)
			}

			opts := CreateOptions{Output: testParams.outOpts}
			for _, file := range files {
				logger := Logger{
					Type: LogFile,
					Options: LogOptions{
						FileName: file.Name(),
						Format:   LogFormatPlain,
					},
				}
				opts.Output.Loggers = append(opts.Output.Loggers, logger)
			}
			opts.Args = testParams.command.args

			cmd, _, err := opts.Resolve(ctx)
			require.NoError(t, err)
			require.NoError(t, cmd.Start())

			cmd.Wait()
			assert.NoError(t, opts.Close())

			for _, file := range files {
				info, err := file.Stat()
				assert.NoError(t, err)
				assert.Equal(t, testParams.numBytesExpected, info.Size())
			}
		})
	}
}
