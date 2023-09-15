package command

import (
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/stretchr/testify/require"
)

func setupTestPatchData(apiData *modelutil.TestModelData, patchPath string, t *testing.T) error {
	if patchPath != "" {
		modulePatchContent, err := os.ReadFile(patchPath)
		require.NoError(t, err)

		patch := &patch.Patch{
			Status:  evergreen.VersionCreated,
			Version: apiData.Task.Version,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		require.NoError(t, patch.Insert())

	}

	return nil
}
