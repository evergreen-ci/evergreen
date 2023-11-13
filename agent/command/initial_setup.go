package command

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/mongodb/jasper"
)

// initialSetup is an internal command used as a placeholder when the agent is
// setting up in preparation to run a task's commands. This is not meant to be
// invoked by end users.
type initialSetup struct{}

func initialSetupFactory() Command                                    { return &initialSetup{} }
func (*initialSetup) Type() string                                    { return evergreen.CommandTypeSystem }
func (*initialSetup) SetType(s string)                                {}
func (*initialSetup) FullDisplayName() string                         { return "initial task setup" }
func (*initialSetup) SetFullDisplayName(s string)                     {}
func (*initialSetup) Name() string                                    { return "setup.initial" }
func (*initialSetup) SetIdleTimeout(d time.Duration)                  {}
func (*initialSetup) IdleTimeout() time.Duration                      { return 0 }
func (*initialSetup) ParseParams(params map[string]interface{}) error { return nil }
func (*initialSetup) JasperManager() jasper.Manager                   { return nil }
func (*initialSetup) SetJasperManager(_ jasper.Manager)               {}
func (*initialSetup) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	logger.Task().Info("Performing initial task setup.")
	return nil
}
