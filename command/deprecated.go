package command

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"golang.org/x/net/context"
)

// gitApplyPatch is deprecated. Its functionality is now a part of GitGetProjectCommand.
type gitApplyPatch struct{ base }

func gitApplyPatchFactory() Command                                    { return &gitApplyPatch{} }
func (*gitApplyPatch) Name() string                                    { return "git.apply_patch" }
func (*gitApplyPatch) ParseParams(params map[string]interface{}) error { return nil }
func (*gitApplyPatch) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Warning("git.apply_patch is deprecated. Patches are applied in git.get_project.")
	return nil
}

// the fetchVars command is deprecated.
type fetchVars struct{ base }

func fetchVarsFactory() Command                                      { return &fetchVars{} }
func (c *fetchVars) Name() string                                    { return "expansions.fetch_vars" }
func (c *fetchVars) ParseParams(params map[string]interface{}) error { return nil }
func (c *fetchVars) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Warning("expansions.fetch deprecated")
	return nil
}

type shellCleanup struct{ base }

func shellCleanupFactory() Command                                       { return &shellCleanup{} }
func (cc *shellCleanup) Name() string                                    { return "shell.cleanup" }
func (cc *shellCleanup) ParseParams(params map[string]interface{}) error { return nil }
func (cc *shellCleanup) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.cleanup is deprecated. Process cleanup is now enabled by default.")
	return nil
}

type shellTrack struct{ base }

func shellTrackFactory() Command                                       { return &shellTrack{} }
func (cc *shellTrack) Name() string                                    { return "shell.track" }
func (cc *shellTrack) ParseParams(params map[string]interface{}) error { return nil }
func (cc *shellTrack) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.track is deprecated. Process tracking is now enabled by default.")
	return nil
}
