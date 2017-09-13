package plugintest

import (
	"io/ioutil"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/patch"
	modelutil "github.com/evergreen-ci/evergreen/model/testutil"
	"github.com/evergreen-ci/evergreen/testutil"
)

func SetupPatchData(apiData *modelutil.TestModelData, patchPath string, t *testing.T) error {

	if patchPath != "" {
		modulePatchContent, err := ioutil.ReadFile(patchPath)
		testutil.HandleTestingErr(err, t, "failed to read test module patch file %v")

		patch := &patch.Patch{
			Status:  evergreen.PatchCreated,
			Version: apiData.TaskConfig.Version.Id,
			Patches: []patch.ModulePatch{
				{
					ModuleName: "enterprise",
					Githash:    "c2d7ce942a96d7dacd27c55b257e3f2774e04abf",
					PatchSet:   patch.PatchSet{Patch: string(modulePatchContent)},
				},
			},
		}

		testutil.HandleTestingErr(patch.Insert(), t, "failed to insert patch %v")

	}

	return nil
}
