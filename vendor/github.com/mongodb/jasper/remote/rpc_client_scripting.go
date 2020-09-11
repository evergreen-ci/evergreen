package remote

import (
	"context"

	internal "github.com/mongodb/jasper/remote/internal"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// rpcScriptingHarness is the client-side representation of a scripting.Harness
// for making requests to the remote gRPC service.
type rpcScriptingHarness struct {
	id     string
	client internal.JasperProcessManagerClient
}

func newRPCScriptingHarness(client internal.JasperProcessManagerClient, id string) *rpcScriptingHarness {
	return &rpcScriptingHarness{
		client: client,
		id:     id,
	}
}

func (s *rpcScriptingHarness) ID() string { return s.id }

func (s *rpcScriptingHarness) Setup(ctx context.Context) error {
	resp, err := s.client.ScriptingHarnessSetup(ctx, &internal.ScriptingHarnessID{Id: s.id})
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}

func (s *rpcScriptingHarness) Run(ctx context.Context, args []string) error {
	resp, err := s.client.ScriptingHarnessRun(ctx, &internal.ScriptingHarnessRunArgs{Id: s.id, Args: args})
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}

func (s *rpcScriptingHarness) RunScript(ctx context.Context, script string) error {
	resp, err := s.client.ScriptingHarnessRunScript(ctx, &internal.ScriptingHarnessRunScriptArgs{Id: s.id, Script: script})
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}

func (s *rpcScriptingHarness) Build(ctx context.Context, dir string, args []string) (string, error) {
	resp, err := s.client.ScriptingHarnessBuild(ctx, &internal.ScriptingHarnessBuildArgs{Id: s.id, Directory: dir, Args: args})
	if err != nil {
		return "", errors.WithStack(err)
	}

	if !resp.Outcome.Success {
		return "", errors.New(resp.Outcome.Text)
	}

	return resp.Path, nil
}

func (s *rpcScriptingHarness) Test(ctx context.Context, dir string, args ...scripting.TestOptions) ([]scripting.TestResult, error) {
	resp, err := s.client.ScriptingHarnessTest(ctx, &internal.ScriptingHarnessTestArgs{Id: s.id, Directory: dir, Options: internal.ConvertScriptingTestOptions(args)})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if !resp.Outcome.Success {
		return nil, errors.New(resp.Outcome.Text)
	}

	return resp.Export()
}

func (s *rpcScriptingHarness) Cleanup(ctx context.Context) error {
	resp, err := s.client.ScriptingHarnessCleanup(ctx, &internal.ScriptingHarnessID{Id: s.id})
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}
