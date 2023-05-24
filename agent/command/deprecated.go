package command

import (
	"context"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
)

// gitApplyPatch is deprecated. Its functionality is now a part of GitGetProjectCommand.
type gitApplyPatch struct{ base }

func gitApplyPatchFactory() Command                                    { return &gitApplyPatch{} }
func (*gitApplyPatch) Name() string                                    { return "git.apply_patch" }
func (*gitApplyPatch) ParseParams(params map[string]interface{}) error { return nil }
func (*gitApplyPatch) Execute(ctx context.Context,
	client client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	logger.Task().Warning("git.apply_patch is deprecated. Patches are applied in git.get_project.")
	return nil
}

type manifestLoad struct{ base }

func manifestLoadFactory() Command                                      { return &manifestLoad{} }
func (c *manifestLoad) Name() string                                    { return "manifest.load" }
func (c *manifestLoad) ParseParams(params map[string]interface{}) error { return nil }
func (c *manifestLoad) Execute(ctx context.Context,
	comm client.Communicator, logger client.LoggerProducer, conf *internal.TaskConfig) error {

	logger.Execution().Warningf("manifest.load is deprecated. Manifest load is now called automatically in git.get_project.")
	return nil
}
