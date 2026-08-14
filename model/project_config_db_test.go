package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindLastKnownGoodProjectConfig(t *testing.T) {
	const projectID = "project-id"

	for testName, testCase := range map[string]struct {
		configs          []ProjectConfig
		expectedID       string
		expectNoDocument bool
	}{
		"NewestEligibleConfigShouldBeSelected": {
			configs: []ProjectConfig{
				{Id: "eligible-a", Project: projectID, Requester: evergreen.RepotrackerVersionRequester, CreateTime: time.Now().Add(-time.Hour)},
				{Id: "eligible-b", Project: projectID, Requester: evergreen.AdHocRequester, CreateTime: time.Now()},
			},
			expectedID: "eligible-b",
		},
		"ConfigRequiringVettingShouldNotBeSelected": {
			configs: []ProjectConfig{
				{Id: "eligible-a", Project: projectID, Requester: evergreen.RepotrackerVersionRequester, CreateTime: time.Now().Add(-time.Hour)},
				{Id: "unvetted", Project: projectID, Requester: evergreen.PatchVersionRequester, CreateTime: time.Now()},
			},
			expectedID: "eligible-a",
		},
		"OnlyUnvettedConfigsShouldReturnNothing": {
			configs: []ProjectConfig{
				{Id: "unvetted", Project: projectID, Requester: evergreen.PatchVersionRequester, CreateTime: time.Now()},
			},
			expectNoDocument: true,
		},
		"ConfigWithoutRequesterShouldStillBeFound": {
			configs: []ProjectConfig{
				{Id: "legacy", Project: projectID, CreateTime: time.Now()},
			},
			expectedID: "legacy",
		},
		"ConfigFromAnotherProjectShouldNotBeReturned": {
			configs: []ProjectConfig{
				{Id: "other", Project: "other-project", Requester: evergreen.RepotrackerVersionRequester, CreateTime: time.Now()},
			},
			expectNoDocument: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectConfigCollection))
			for i := range testCase.configs {
				require.NoError(t, testCase.configs[i].Insert(t.Context()))
			}

			pc, err := FindLastKnownGoodProjectConfig(t.Context(), projectID)
			require.NoError(t, err)
			if testCase.expectNoDocument {
				assert.Nil(t, pc)
				return
			}
			require.NotNil(t, pc)
			assert.Equal(t, testCase.expectedID, pc.Id)
		})
	}

	// Verify the behavior is uniform for every requester that requires vetting.
	for _, requester := range evergreen.PatchRequesters {
		t.Run("UnvettedConfigShouldNotBeSelected", func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ProjectConfigCollection))
			eligible := ProjectConfig{Id: "eligible", Project: projectID, Requester: evergreen.RepotrackerVersionRequester, CreateTime: time.Now().Add(-time.Hour)}
			unvetted := ProjectConfig{Id: "unvetted", Project: projectID, Requester: requester, CreateTime: time.Now()}
			require.NoError(t, eligible.Insert(t.Context()))
			require.NoError(t, unvetted.Insert(t.Context()))

			pc, err := FindLastKnownGoodProjectConfig(t.Context(), projectID)
			require.NoError(t, err)
			require.NotNil(t, pc)
			assert.Equal(t, "eligible", pc.Id)
		})
	}
}

func TestFindProjectConfigForProjectOrVersionByIdAndDefault(t *testing.T) {
	const projectID = "project-id"
	require.NoError(t, db.ClearCollections(ProjectConfigCollection))

	eligible := ProjectConfig{Id: "eligible", Project: projectID, Requester: evergreen.RepotrackerVersionRequester, CreateTime: time.Now().Add(-time.Hour)}
	unvetted := ProjectConfig{Id: "unvetted", Project: projectID, Requester: evergreen.PatchVersionRequester, CreateTime: time.Now()}
	require.NoError(t, eligible.Insert(t.Context()))
	require.NoError(t, unvetted.Insert(t.Context()))

	// A config is still retrievable by its own ID even when it is not eligible for selection.
	pc, err := FindProjectConfigForProjectOrVersion(t.Context(), projectID, "unvetted")
	require.NoError(t, err)
	require.NotNil(t, pc)
	assert.Equal(t, "unvetted", pc.Id)

	pc, err = FindProjectConfigForProjectOrVersion(t.Context(), projectID, "")
	require.NoError(t, err)
	require.NotNil(t, pc)
	assert.Equal(t, "eligible", pc.Id)
}
