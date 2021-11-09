package cli

import (
	"context"
	"encoding/json"

	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// sshScriptingHarness is the client-side representation of a
// scripting.Harness for making requests to remote services via the CLI over
// SSH.
type sshScriptingHarness struct {
	ctx    context.Context
	id     string
	client *sshRunner
}

func newSSHScriptingHarness(ctx context.Context, client *sshRunner, id string) *sshScriptingHarness {
	return &sshScriptingHarness{
		ctx:    ctx,
		id:     id,
		client: client,
	}
}

func (s *sshScriptingHarness) ID() string { return s.id }

func (s *sshScriptingHarness) Setup(ctx context.Context) error {
	output, err := s.runCommand(s.ctx, ScriptingSetupCommand, IDInput{ID: s.id})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading scripting harness response")
	}

	return nil
}

func (s *sshScriptingHarness) Run(ctx context.Context, args []string) error {
	output, err := s.runCommand(s.ctx, ScriptingRunCommand, ScriptingRunInput{ID: s.id, Args: args})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading scripting harness response")
	}

	return nil
}

func (s *sshScriptingHarness) RunScript(ctx context.Context, script string) error {
	output, err := s.runCommand(s.ctx, ScriptingRunScriptCommand, ScriptingRunScriptInput{ID: s.id, Script: script})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading scripting harness response")
	}

	return nil
}

func (s *sshScriptingHarness) Build(ctx context.Context, dir string, args []string) (string, error) {
	output, err := s.runCommand(s.ctx, ScriptingBuildCommand, ScriptingBuildInput{
		ID:        s.id,
		Directory: dir,
		Args:      args,
	})
	if err != nil {
		return "", errors.Wrap(err, "running command")
	}

	resp, err := ExtractScriptingBuildResponse(output)
	if err != nil {
		return "", errors.Wrap(err, "reading scripting harness response")
	}

	return resp.Path, nil
}

func (s *sshScriptingHarness) Test(ctx context.Context, dir string, opts ...scripting.TestOptions) ([]scripting.TestResult, error) {
	output, err := s.runCommand(s.ctx, ScriptingTestCommand, ScriptingTestInput{
		ID:        s.id,
		Directory: dir,
		Options:   opts,
	})
	if err != nil {
		return nil, errors.Wrap(err, "running command")
	}

	resp, err := ExtractScriptingTestResponse(output)
	if err != nil {
		return nil, errors.Wrap(err, "reading scripting harness response")
	}
	return resp.Results, nil
}

func (s *sshScriptingHarness) Cleanup(ctx context.Context) error {
	output, err := s.runCommand(s.ctx, ScriptingCleanupCommand, IDInput{ID: s.id})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err := ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading scripting harness response")
	}
	return nil
}

func (s *sshScriptingHarness) runCommand(ctx context.Context, scriptingSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return s.client.runClientCommand(ctx, []string{ScriptingCommand, scriptingSubcommand}, subcommandInput)
}
