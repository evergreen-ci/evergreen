package manifest

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestFindFromVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, patch.Collection))

	moduleName := "sample_module"
	projectName := "p1"
	revision := "12345"
	mfest := &Manifest{
		Id:          "m1",
		ProjectName: projectName,
		Revision:    revision,
		Modules:     map[string]*Module{moduleName: &Module{}},
	}
	_, err := mfest.TryInsert()
	assert.NoError(t, err)

	patchID := "aabbccddeeff001122334455"
	p := patch.Patch{
		Id: patch.NewId(patchID),
		Patches: []patch.ModulePatch{
			{
				ModuleName: moduleName,
				Githash:    "abcdef",
			},
		},
	}
	assert.NoError(t, p.Insert())

	mfest, err = FindFromVersion(patchID, projectName, revision, evergreen.PatchVersionRequester)
	assert.NoError(t, err)
	assert.Equal(t, "abcdef", mfest.ModuleOverrides[moduleName])
}
