package command

import (
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/rest/client"
	"golang.org/x/net/context"
)

// gitApplyPatch is deprecated. Its functionality is now a part of GitGetProjectCommand.
type gitApplyPatch struct{}

func gitApplyPatchFactory() Command                                    { return &gitApplyPatch{} }
func (*gitApplyPatch) Name() string                                    { return "apply_patch" }
func (*gitApplyPatch) Plugin() string                                  { return "git" }
func (*gitApplyPatch) ParseParams(params map[string]interface{}) error { return nil }
func (*gitApplyPatch) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Warning("git.apply_patch is deprecated. Patches are applied in git.get_project.")
	return nil
}

// the fetchVars command is deprecated.
type fetchVars struct{}

func fetchVarsFactory() Command                                      { return &fetchVars{} }
func (c *fetchVars) Name() string                                    { return "fetch_vars" }
func (c *fetchVars) Plugin() string                                  { return "expansions" }
func (c *fetchVars) ParseParams(params map[string]interface{}) error { return nil }
func (c *fetchVars) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Task().Warning("expansions.fetch deprecated")
	return nil
}

type shellCleanup struct{}

func shellCleanupFactory() Command                                       { return &shellCleanup{} }
func (cc *shellCleanup) Name() string                                    { return "cleanup" }
func (cc *shellCleanup) Plugin() string                                  { return "shell" }
func (cc *shellCleanup) ParseParams(params map[string]interface{}) error { return nil }
func (cc *shellCleanup) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.cleanup is deprecated. Process cleanup is now enabled by default.")
	return nil
}

type shellTrack struct{}

func shellTrackFactory() Command                                       { return &shellTrack{} }
func (cc *shellTrack) Name() string                                    { return "track" }
func (cc *shellTrack) Plugin() string                                  { return "shell" }
func (cc *shellTrack) ParseParams(params map[string]interface{}) error { return nil }
func (cc *shellTrack) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *model.TaskConfig) error {

	logger.Execution().Warning("shell.track is deprecated. Process tracking is now enabled by default.")
	return nil
}
