package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// sshManager uses SSH to access a remote machine's Jasper CLI, which has access to
// methods in the Manager interface.
type sshManager struct {
	manager jasper.Manager
	opts    sshClientOptions
}

// NewSSHManager creates a new Jasper manager that connects to a remote
// machine's Jasper service over SSH using the remote machine's Jasper CLI.
func NewSSHManager(remoteOpts jasper.RemoteOptions, clientOpts ClientOptions, trackProcs bool) (jasper.Manager, error) {
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

	return &sshManager{
		opts: sshClientOptions{
			Machine: remoteOpts,
			Client:  clientOpts,
		},
		manager: manager,
	}, nil
}

func (m *sshManager) CreateProcess(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
	output, err := m.runCommand(ctx, CreateProcessCommand, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(m.runClientCommand, resp.Info)
}

// CreateCommand creates an in-memory command whose subcommands run over SSH.
// However, the desired semantics would be to actually send CommandInput to the
// Jasper CLI over SSH.
// TODO: this can likely be fixed by serializing the command inputs, which
// requires MAKE-841.
func (m *sshManager) CreateCommand(ctx context.Context) *jasper.Command {
	return m.manager.CreateCommand(ctx).ProcConstructor(m.CreateProcess)
}

func (m *sshManager) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register existing processes on remote manager")
}

func (m *sshManager) List(ctx context.Context, f jasper.Filter) ([]jasper.Process, error) {
	output, err := m.runCommand(ctx, ListCommand, &FilterInput{Filter: f})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfosResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	procs := make([]jasper.Process, len(resp.Infos))
	for i := range resp.Infos {
		if procs[i], err = newSSHProcess(m.runClientCommand, resp.Infos[i]); err != nil {
			return nil, errors.Wrap(err, "problem creating SSH process")
		}
	}

	return procs, nil
}

func (m *sshManager) Group(ctx context.Context, tag string) ([]jasper.Process, error) {
	output, err := m.runCommand(ctx, GroupCommand, &TagInput{Tag: tag})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfosResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	procs := make([]jasper.Process, len(resp.Infos))
	for i := range resp.Infos {
		if procs[i], err = newSSHProcess(m.runClientCommand, resp.Infos[i]); err != nil {
			return nil, errors.Wrap(err, "problem creating SSH process")
		}
	}

	return procs, nil
}

func (m *sshManager) Get(ctx context.Context, id string) (jasper.Process, error) {
	output, err := m.runCommand(ctx, GetCommand, &IDInput{ID: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(m.runClientCommand, resp.Info)
}

func (m *sshManager) Clear(ctx context.Context) {
	_, _ = m.runCommand(ctx, ClearCommand, nil)
}

func (m *sshManager) Close(ctx context.Context) error {
	output, err := m.runCommand(ctx, CloseCommand, nil)
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (m *sshManager) runCommand(ctx context.Context, managerSubcommand string, subcommandInput interface{}) ([]byte, error) {
	return m.runClientCommand(ctx, []string{ManagerCommand, managerSubcommand}, subcommandInput)
}

// runClientCommand creates a command that runs the given CLI client subcommand
// over SSH with the given input to be sent as JSON to standard input. If
// subcommandInput is nil, it does not use standard input.
func (m *sshManager) runClientCommand(ctx context.Context, subcommand []string, subcommandInput interface{}) ([]byte, error) {
	input, err := clientInput(subcommandInput)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating client input")
	}
	output := clientOutput()

	cmd := m.newClientCommand(ctx, subcommand, input, output)
	if err := cmd.Run(ctx); err != nil {
		return nil, errors.Wrapf(err, "problem running command '%s' over SSH", m.opts.buildCommand(subcommand...))
	}

	return output.Bytes(), nil
}

// newClientCommand creates the command that runs the Jasper CLI client command
// over SSH.
func (m *sshManager) newClientCommand(ctx context.Context, clientSubcommand []string, input io.Reader, output io.WriteCloser) *jasper.Command {
	cmd := m.manager.CreateCommand(ctx).Host(m.opts.Machine.Host).User(m.opts.Machine.User).ExtendRemoteArgs(m.opts.Machine.Args...).
		Add(m.opts.buildCommand(clientSubcommand...))

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
