package cli

import (
	"context"

	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
)

// sshClientScriptingHarness is the client-side representation of a
// scripting.Harness for making requests to remote services via the CLI over
// SSH.
type sshClientScriptingHarness struct {
	id     string
	client *sshRunner
}

func newSSHClientScriptingHarness(client *sshRunner, id string) *sshClientScriptingHarness {
	return &sshClientScriptingHarness{
		id:     id,
		client: client,
	}
}

func (s *sshClientScriptingHarness) ID() string { return s.id }

// TODO (EVG-12913): implement

func (s *sshClientScriptingHarness) Setup(ctx context.Context) error {
	return errors.New("not implemented")
}

func (s *sshClientScriptingHarness) Run(ctx context.Context, args []string) error {
	return errors.New("not implemented")
}

func (s *sshClientScriptingHarness) RunScript(ctx context.Context, script string) error {
	return errors.New("not implemented")
}

func (s *sshClientScriptingHarness) Build(ctx context.Context, dir string, args []string) (string, error) {
	return "", errors.New("not implemented")
}

func (s *sshClientScriptingHarness) Test(ctx context.Context, dir string, opts ...scripting.TestOptions) ([]scripting.TestResult, error) {
	return nil, errors.New("not implemented")
}

func (s *sshClientScriptingHarness) Cleanup(ctx context.Context) error {
	return errors.New("not implemented")
}
