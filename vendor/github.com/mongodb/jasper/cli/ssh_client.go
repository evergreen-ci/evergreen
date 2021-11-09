package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/remote"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// sshClient uses SSH to access a remote machine's Jasper CLI, which has access
// to methods in the remote.Manager interface.
type sshClient struct {
	client *sshRunner
}

// NewSSHClient creates a new Jasper manager that connects to a remote
// machine's Jasper service over SSH using the remote machine's Jasper CLI.
func NewSSHClient(clientOpts ClientOptions, remoteOpts options.Remote) (remote.Manager, error) {
	if err := remoteOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem validating remote options")
	}
	for _, arg := range remoteOpts.Args {
		if strings.HasPrefix(arg, "-v") {
			return nil, errors.New("cannot use verbose arguments in non-interactive SSH client")
		}
	}
	// We have to run SSH without output, because it will prevent the JSON
	// output from the Jasper CLI from being parsed correctly (e.g. adding a
	// host to the known hosts file generates a warning).
	remoteOpts.Args = append(remoteOpts.Args,
		"-T",
		"-o", "LogLevel=QUIET",
	)

	if err := clientOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem validating client options")
	}

	runner, err := newSSHRunner(clientOpts, remoteOpts)
	if err != nil {
		return nil, errors.Wrap(err, "could not set up SSH client")
	}
	return &sshClient{
		client: runner,
	}, nil
}

func (c *sshClient) ID() string {
	output, err := c.runManagerCommand(context.Background(), IDCommand, nil)
	if err != nil {
		return ""
	}

	resp, err := ExtractIDResponse(output)
	if err != nil {
		return ""
	}

	return resp.ID
}

func (c *sshClient) CreateProcess(ctx context.Context, opts *options.Create) (jasper.Process, error) {
	output, err := c.runManagerCommand(ctx, CreateProcessCommand, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(c.client, resp.Info)
}

// CreateCommand creates a command that logically will execute via the remote
// CLI. Users should not use (*jasper.Command).SetRunFunc().
func (c *sshClient) CreateCommand(ctx context.Context) *jasper.Command {
	return c.client.manager.CreateCommand(ctx).SetRunFunc(func(opts options.Command) error {
		output, err := c.runManagerCommand(ctx, CreateCommand, &opts)
		if err != nil {
			return errors.Wrap(err, "could not run command from given input")
		}

		if _, err := ExtractOutcomeResponse(output); err != nil {
			return errors.WithStack(err)
		}

		return nil
	})
}

func (c *sshClient) CreateScripting(ctx context.Context, opts options.ScriptingHarness) (scripting.Harness, error) {
	output, err := c.runRemoteCommand(ctx, CreateScriptingCommand, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractIDResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHScriptingHarness(ctx, c.client, resp.ID), nil
}

func (c *sshClient) GetScripting(ctx context.Context, id string) (scripting.Harness, error) {
	output, err := c.runRemoteCommand(ctx, GetScriptingCommand, &IDInput{ID: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHScriptingHarness(ctx, c.client, id), nil
}

func (c *sshClient) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register existing processes on remote manager")
}

func (c *sshClient) List(ctx context.Context, f options.Filter) ([]jasper.Process, error) {
	output, err := c.runManagerCommand(ctx, ListCommand, &FilterInput{Filter: f})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfosResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	procs := make([]jasper.Process, len(resp.Infos))
	for i := range resp.Infos {
		if procs[i], err = newSSHProcess(c.client, resp.Infos[i]); err != nil {
			return nil, errors.Wrap(err, "problem creating SSH process")
		}
	}

	return procs, nil
}

func (c *sshClient) Group(ctx context.Context, tag string) ([]jasper.Process, error) {
	output, err := c.runManagerCommand(ctx, GroupCommand, &TagInput{Tag: tag})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfosResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	procs := make([]jasper.Process, len(resp.Infos))
	for i := range resp.Infos {
		if procs[i], err = newSSHProcess(c.client, resp.Infos[i]); err != nil {
			return nil, errors.Wrap(err, "problem creating SSH process")
		}
	}

	return procs, nil
}

func (c *sshClient) Get(ctx context.Context, id string) (jasper.Process, error) {
	output, err := c.runManagerCommand(ctx, GetCommand, &IDInput{ID: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(c.client, resp.Info)
}

func (c *sshClient) Clear(ctx context.Context) {
	if _, err := c.runManagerCommand(ctx, ClearCommand, nil); err != nil {
		grip.Debug(errors.Wrap(err, "clearing manager"))
	}
}

func (c *sshClient) Close(ctx context.Context) error {
	output, err := c.runManagerCommand(ctx, CloseCommand, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) WriteFile(ctx context.Context, opts options.WriteFile) error {
	return opts.WriteBufferedContent(func(opts options.WriteFile) error {
		output, err := c.runManagerCommand(ctx, WriteFileCommand, &opts)
		if err != nil {
			return errors.WithStack(err)
		}

		if _, err := ExtractOutcomeResponse(output); err != nil {
			return errors.WithStack(err)
		}

		return nil
	})
}

func (c *sshClient) CloseConnection() error {
	return nil
}

func (c *sshClient) ConfigureCache(ctx context.Context, opts options.Cache) error {
	output, err := c.runRemoteCommand(ctx, ConfigureCacheCommand, &opts)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) DownloadFile(ctx context.Context, opts options.Download) error {
	output, err := c.runRemoteCommand(ctx, DownloadFileCommand, &opts)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	output, err := c.runRemoteCommand(ctx, DownloadMongoDBCommand, &opts)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	output, err := c.runRemoteCommand(ctx, GetLogStreamCommand, &LogStreamInput{ID: id, Count: count})
	if err != nil {
		return jasper.LogStream{}, errors.WithStack(err)
	}

	resp, err := ExtractLogStreamResponse(output)
	if err != nil {
		return resp.LogStream, errors.WithStack(err)
	}

	return resp.LogStream, nil
}

func (c *sshClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	output, err := c.runRemoteCommand(ctx, GetBuildloggerURLsCommand, &IDInput{ID: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractBuildloggerURLsResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return resp.URLs, nil
}

func (c *sshClient) SignalEvent(ctx context.Context, name string) error {
	output, err := c.runRemoteCommand(ctx, SignalEventCommand, &EventInput{Name: name})
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) LoggingCache(ctx context.Context) jasper.LoggingCache {
	return newSSHLoggingCache(ctx, c.client)
}

func (c *sshClient) SendMessages(ctx context.Context, opts options.LoggingPayload) error {
	output, err := c.runRemoteCommand(ctx, SendMessagesCommand, opts)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) runManagerCommand(ctx context.Context, managerSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return c.client.runClientCommand(ctx, []string{ManagerCommand, managerSubcommand}, subcommandInput)
}

func (c *sshClient) runRemoteCommand(ctx context.Context, remoteSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return c.client.runClientCommand(ctx, []string{RemoteCommand, remoteSubcommand}, subcommandInput)
}

// sshRunner is a client to help run Jasper CLI commands over SSH.
type sshRunner struct {
	manager    jasper.Manager
	clientOpts ClientOptions
	remoteOpts options.Remote
}

func newSSHRunner(clientOpts ClientOptions, remoteOpts options.Remote) (*sshRunner, error) {
	manager, err := jasper.NewSynchronizedManager(false)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating underlying manager")
	}

	return &sshRunner{
		clientOpts: clientOpts,
		remoteOpts: remoteOpts,
		manager:    manager,
	}, nil
}

// runClientCommand creates a command that runs the given CLI client subcommand
// over SSH with the given input to be sent as JSON to standard input. If
// subcommandInput is nil, it does not use standard input.
func (r *sshRunner) runClientCommand(ctx context.Context, subcommand []string, subcommandInput interface{}) (json.RawMessage, error) {
	input, err := clientInput(subcommandInput)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating client input")
	}
	output := clientOutput()

	cmd := r.newCommand(ctx, subcommand, input, output)
	if err := cmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "problem running command '%s' over SSH", r.clientOpts.buildCommand(subcommand...))
	}

	return output.Bytes(), nil
}

// newCommand creates the command that runs the Jasper CLI client command
// over SSH.
func (r *sshRunner) newCommand(ctx context.Context, clientSubcommand []string, input json.RawMessage, output io.WriteCloser) *jasper.Command {
	cmd := r.manager.CreateCommand(ctx).SetRemoteOptions(&r.remoteOpts).
		Add(r.clientOpts.buildCommand(clientSubcommand...))

	if len(input) != 0 {
		cmd.SetInputBytes(input)
	}

	if output != nil {
		cmd.SetCombinedWriter(output)
	}

	return cmd
}

// clientOutput constructs the buffer to write the CLI output.
func clientOutput() *CappedWriter {
	return &CappedWriter{
		Buffer:   &bytes.Buffer{},
		MaxBytes: 1024 * 1024, // 1 MB
	}
}

// clientInput constructs the JSON input to the CLI from the struct.
func clientInput(input interface{}) (json.RawMessage, error) {
	if input == nil {
		return nil, nil
	}

	inputBytes, err := json.MarshalIndent(input, "", "    ")
	if err != nil {
		return nil, errors.Wrap(err, "could not encode input as JSON")
	}

	return inputBytes, nil
}
