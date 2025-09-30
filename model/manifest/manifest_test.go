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

	manifestID1 := "aabbccddeeff001122334411"
	manifestID2 := "aabbccddeeff001122334422"

	mfests := []Manifest{
		{
			Id:          manifestID1,
			ProjectName: projectName,
			Revision:    revision,
			Modules:     map[string]*Module{moduleName: {}},
			IsBase:      true,
		},
		{
			Id:          manifestID2,
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

	// Should apply module overrides from patch
	mfest, err := FindFromVersion(t.Context(), patchID, projectName, revision, evergreen.PatchVersionRequester)
	assert.NoError(t, err)
	assert.Equal(t, manifestID1, mfest.Id)
	assert.Equal(t, "abcdef", mfest.ModuleOverrides[moduleName])

	// Since there's no patch with ID manifestID2, this should return an error
	_, err = FindFromVersion(t.Context(), manifestID2, projectName, revision, evergreen.PatchVersionRequester)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no corresponding patch")

	p2 := patch.Patch{
		Id: patch.NewId(manifestID2), 
		Patches: []patch.ModulePatch{
			{
				ModuleName: moduleName,
				Githash:    "fedcba",
			},
		},
	}
	assert.NoError(t, p2.Insert(t.Context()))

	// Now finding the manifest with its ID as a patch should apply module overrides
	mfest3, err := FindFromVersion(t.Context(), manifestID2, projectName, revision, evergreen.PatchVersionRequester)
	assert.NoError(t, err)
	assert.Equal(t, manifestID2, mfest3.Id)
	assert.Equal(t, "fedcba", mfest3.ModuleOverrides[moduleName])

	// Non-patch requester should not get module overrides
	mfest4, err := FindFromVersion(t.Context(), manifestID2, projectName, revision, evergreen.RepotrackerVersionRequester)
	assert.NoError(t, err)
	assert.Equal(t, manifestID2, mfest4.Id)
	assert.Empty(t, mfest4.ModuleOverrides)

	// If no manifest exists for this version we get the base version's manifest
	mfest5, err := FindFromVersion(t.Context(), "deadbeefdeadbeefdeadbeef", projectName, revision, "")
	assert.NoError(t, err)
	assert.Equal(t, manifestID1, mfest5.Id)
	assert.Empty(t, mfest5.ModuleOverrides)
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
