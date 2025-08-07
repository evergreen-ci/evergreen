package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/manifest"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFromService(t *testing.T) {
	mfst := &manifest.Manifest{
		Id:          "test-manifest",
		Revision:    "1234567890abcdef",
		ProjectName: "test-project",
		Branch:      "main",
		Modules: map[string]*manifest.Module{
			"module1": {
				Branch:   "main",
				Owner:    "owner",
				Repo:     "repo",
				Revision: "abcdef1234567890",
				URL:      "url",
			},
		},
	}
	apiManifest := &APIManifest{}
	apiManifest.BuildFromService(mfst)

	assert.Equal(t, mfst.Id, utility.FromStringPtr(apiManifest.Id))
	assert.Equal(t, mfst.Revision, utility.FromStringPtr(apiManifest.Revision))
	assert.Equal(t, mfst.ProjectName, utility.FromStringPtr(apiManifest.ProjectName))
	assert.Equal(t, mfst.Branch, utility.FromStringPtr(apiManifest.Branch))
	require.Len(t, apiManifest.Modules, 1)
	expectedModule := mfst.Modules["module1"]
	assert.Equal(t, "module1", utility.FromStringPtr(apiManifest.Modules[0].Name))
	assert.Equal(t, expectedModule.Branch, utility.FromStringPtr(apiManifest.Modules[0].Branch))
	assert.Equal(t, expectedModule.Owner, utility.FromStringPtr(apiManifest.Modules[0].Owner))
	assert.Equal(t, expectedModule.Repo, utility.FromStringPtr(apiManifest.Modules[0].Repo))
	assert.Equal(t, expectedModule.Revision, utility.FromStringPtr(apiManifest.Modules[0].Revision))
	assert.Equal(t, expectedModule.URL, utility.FromStringPtr(apiManifest.Modules[0].URL))
}
