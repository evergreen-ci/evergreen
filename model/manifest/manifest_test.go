package manifest

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/patch"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindFromVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, patch.Collection))

	moduleName := "sample_module"
	projectName := "p1"
	revision := "12345"
	mfests := []Manifest{
		{
			Id:          "m1",
			ProjectName: projectName,
			Revision:    revision,
			Modules:     map[string]*Module{moduleName: {}},
			IsBase:      true,
		},
		{
			Id:          "m2",
			ProjectName: projectName,
			Revision:    revision,
			Modules:     map[string]*Module{moduleName: {}},
		},
	}
	for _, mfest := range mfests {
		_, err := mfest.TryInsert(t.Context())
		assert.NoError(t, err)
	}

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
	assert.NoError(t, p.Insert(t.Context()))

	mfest, err := FindFromVersion(t.Context(), patchID, projectName, revision, evergreen.PatchVersionRequester)
	assert.NoError(t, err)
	assert.Equal(t, "m1", mfest.Id)
	assert.Equal(t, "abcdef", mfest.ModuleOverrides[moduleName])

	// If no manifest exists for this version we get the base version's manifest
	mfest, err = FindFromVersion(t.Context(), "deadbeefdeadbeefdeadbeef", projectName, revision, "")
	assert.NoError(t, err)
	assert.Equal(t, "m1", mfest.Id)
}

func TestByBaseProjectAndRevision(t *testing.T) {
	require.NoError(t, db.Clear(Collection))
	mfests := []Manifest{
		{
			Id:          "m1",
			ProjectName: "evergreen",
			Revision:    "abcdef",
			IsBase:      true,
		},
		{
			Id:          "m2",
			ProjectName: "evergreen",
			Revision:    "abcdef",
		},
	}
	for _, mfest := range mfests {
		_, err := mfest.TryInsert(t.Context())
		require.NoError(t, err)
	}

	mfest, err := FindOne(t.Context(), ByBaseProjectAndRevision("evergreen", "abcdef"))
	assert.NoError(t, err)
	assert.Equal(t, "m1", mfest.Id)
}
