package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// sshClient uses SSH to access a remote machine's Jasper CLI, which has access
// to methods in the RemoteClient interface.
type sshClient struct {
	manager jasper.Manager
	opts    sshClientOptions
}

// NewSSHClient creates a new Jasper manager that connects to a remote
// machine's Jasper service over SSH using the remote machine's Jasper CLI.
func NewSSHClient(remoteOpts jasper.RemoteOptions, clientOpts ClientOptions, trackProcs bool) (jasper.RemoteClient, error) {
	if err := remoteOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem validating remote options")
	}

	if err := clientOpts.Validate(); err != nil {
		return nil, errors.Wrap(err, "problem validating client options")
	}

	manager, err := jasper.NewLocalManager(trackProcs)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating underlying manager")
	}

	return &sshClient{
		opts: sshClientOptions{
			Machine: remoteOpts,
			Client:  clientOpts,
		},
		manager: manager,
	}, nil
}

func (c *sshClient) CreateProcess(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
	output, err := c.runManagerCommand(ctx, CreateProcessCommand, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(c.runClientCommand, resp.Info)
}

// CreateCommand creates an in-memory command whose subcommands run over SSH.
// However, the desired semantics would be to actually send CommandInput to the
// Jasper CLI over SSH.
// TODO: this can likely be fixed by serializing the command inputs, which
// requires MAKE-841.
func (c *sshClient) CreateCommand(ctx context.Context) *jasper.Command {
	return c.manager.CreateCommand(ctx).ProcConstructor(c.CreateProcess)
}

func (c *sshClient) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register existing processes on remote manager")
}

func (c *sshClient) List(ctx context.Context, f jasper.Filter) ([]jasper.Process, error) {
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
		if procs[i], err = newSSHProcess(c.runClientCommand, resp.Infos[i]); err != nil {
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
		if procs[i], err = newSSHProcess(c.runClientCommand, resp.Infos[i]); err != nil {
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

	return newSSHProcess(c.runClientCommand, resp.Info)
}

func (c *sshClient) Clear(ctx context.Context) {
	_, _ = c.runManagerCommand(ctx, ClearCommand, nil)
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

func (c *sshClient) CloseConnection() error {
	return errors.New("cannot close connection on an SSH client")
}

func (c *sshClient) ConfigureCache(ctx context.Context, opts jasper.CacheOptions) error {
	output, err := c.runRemoteCommand(ctx, ConfigureCacheCommand, &opts)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) DownloadFile(ctx context.Context, info jasper.DownloadInfo) error {
	output, err := c.runRemoteCommand(ctx, DownloadFileCommand, &info)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (c *sshClient) DownloadMongoDB(ctx context.Context, opts jasper.MongoDBDownloadOptions) error {
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

func (c *sshClient) runManagerCommand(ctx context.Context, managerSubcommand string, subcommandInput interface{}) ([]byte, error) {
	return c.runClientCommand(ctx, []string{ManagerCommand, managerSubcommand}, subcommandInput)
}

func (c *sshClient) runRemoteCommand(ctx context.Context, remoteSubcommand string, subcommandInput interface{}) ([]byte, error) {
	return c.runClientCommand(ctx, []string{RemoteCommand, remoteSubcommand}, subcommandInput)
}

// runClientCommand creates a command that runs the given CLI client subcommand
// over SSH with the given input to be sent as JSON to standard input. If
// subcommandInput is nil, it does not use standard input.
func (c *sshClient) runClientCommand(ctx context.Context, subcommand []string, subcommandInput interface{}) ([]byte, error) {
	input, err := clientInput(subcommandInput)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating client input")
	}
	output := clientOutput()

	cmd := c.newCommand(ctx, subcommand, input, output)
	if err := cmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "problem running command '%s' over SSH", c.opts.buildCommand(subcommand...))
	}

	return output.Bytes(), nil
}

// newCommand creates the command that runs the Jasper CLI client command
// over SSH.
func (c *sshClient) newCommand(ctx context.Context, clientSubcommand []string, input io.Reader, output io.WriteCloser) *jasper.Command {
	cmd := c.manager.CreateCommand(ctx).Host(c.opts.Machine.Host).User(c.opts.Machine.User).ExtendRemoteArgs(c.opts.Machine.Args...).
		Add(c.opts.buildCommand(clientSubcommand...))

	if input != nil {
		cmd.SetInput(input)
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
func clientInput(input interface{}) (*bytes.Buffer, error) {
	if input == nil {
		return nil, nil
	}

	inputBytes, err := json.MarshalIndent(input, "", "    ")
	if err != nil {
		return nil, errors.Wrap(err, "could not encode input as JSON")
	}

	return bytes.NewBuffer(inputBytes), nil
}
