package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHourlyPatchTaskLimitForProject(t *testing.T) {
	for tName, tCase := range map[string]struct {
		config          TaskLimitsConfig
		projectID       string
		repoRefID       string
		expectedLimit   int
		expectedScopeID string
	}{
		"NoOverridesUsesGeneralLimit": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
			},
			projectID:       "project",
			expectedLimit:   15000,
			expectedScopeID: "",
		},
		"ProjectOverrideAppliesToItsOwnProject": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "project",
						MaxHourlyPatchTasks: 40000,
					},
					{
						ProjectOrRepoID:     "other_project",
						MaxHourlyPatchTasks: 30000,
					},
				},
			},
			projectID:       "project",
			expectedLimit:   40000,
			expectedScopeID: "project",
		},
		"ProjectOverrideDoesNotApplyToOtherProjects": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "other_project",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			expectedLimit:   15000,
			expectedScopeID: "",
		},
		"RepoOverrideAppliesToProjectTrackingTheRepo": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "repo",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			repoRefID:       "repo",
			expectedLimit:   40000,
			expectedScopeID: "repo",
		},
		"RepoOverrideDoesNotApplyToProjectTrackingAnotherRepo": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "other_project",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			repoRefID:       "other_repo",
			expectedLimit:   15000,
			expectedScopeID: "",
		},
		"RepoOverrideIsIgnoredWhenProjectTracksNoRepo": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "repo",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			repoRefID:       "",
			expectedLimit:   15000,
			expectedScopeID: "",
		},
		"ProjectOverrideTakesPrecedenceOverRepoOverrideWhenBothCouldApply": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "repo",
						MaxHourlyPatchTasks: 40000,
					},
					{
						ProjectOrRepoID:     "project",
						MaxHourlyPatchTasks: 20000,
					},
				},
			},
			projectID:       "project",
			repoRefID:       "repo",
			expectedLimit:   20000,
			expectedScopeID: "project",
		},
		"OverrideStillAppliesWhenDefaultLimitIsDisabled": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 0,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "project",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			expectedLimit:   40000,
			expectedScopeID: "project",
		},
		"DisabledDefaultLimitEnforcesNoLimitOnProjectWithoutAnOverride": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 0,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "other_project",
						MaxHourlyPatchTasks: 40000,
					},
				},
			},
			projectID:       "project",
			expectedLimit:   0,
			expectedScopeID: "",
		},
		"OverrideCanLowerTheLimitForItsProject": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
				HourlyPatchTaskOverrides: []HourlyPatchTaskOverride{
					{
						ProjectOrRepoID:     "project",
						MaxHourlyPatchTasks: 100,
					},
				},
			},
			projectID:       "project",
			expectedLimit:   100,
			expectedScopeID: "project",
		},
	} {
		t.Run(tName, func(t *testing.T) {
			limit, scopeID := tCase.config.HourlyPatchTaskLimitForProject(tCase.projectID, tCase.repoRefID)
			assert.Equal(t, tCase.expectedLimit, limit)
			assert.Equal(t, tCase.expectedScopeID, scopeID)
		})
	}
}

func TestTaskLimitsConfigValidateAndDefault(t *testing.T) {
	for tName, tCase := range map[string]struct {
		overrides   []HourlyPatchTaskOverride
		expectedErr string
	}{
		"NoOverridesShouldSucceed": {
			overrides: nil,
		},
		"ProjectAndRepoOverridesShouldSucceed": {
			overrides: []HourlyPatchTaskOverride{
				{
					ProjectOrRepoID:     "project",
					MaxHourlyPatchTasks: 40000,
				},
				{
					ProjectOrRepoID:     "repo",
					MaxHourlyPatchTasks: 30000,
				},
			},
		},
		"OverrideWithoutAProjectOrRepoIDShouldError": {
			overrides: []HourlyPatchTaskOverride{
				{
					MaxHourlyPatchTasks: 40000,
				},
			},
			expectedErr: "must set a project/repo ID",
		},
		"DuplicateOverridesShouldError": {
			overrides: []HourlyPatchTaskOverride{
				{
					ProjectOrRepoID:     "project",
					MaxHourlyPatchTasks: 40000,
				},
				{
					ProjectOrRepoID:     "project",
					MaxHourlyPatchTasks: 30000,
				},
			},
			expectedErr: "duplicate hourly patch task limit override for project/repo 'project'",
		},
		"NegativeOverrideLimitShouldError": {
			overrides: []HourlyPatchTaskOverride{
				{
					ProjectOrRepoID:     "project",
					MaxHourlyPatchTasks: -1,
				},
			},
			expectedErr: "must be positive",
		},
		"ZeroOverrideLimitShouldError": {
			overrides: []HourlyPatchTaskOverride{
				{
					ProjectOrRepoID:     "project",
					MaxHourlyPatchTasks: 0,
				},
			},
			expectedErr: "must be positive",
		},
	} {
		t.Run(tName, func(t *testing.T) {
			c := TaskLimitsConfig{HourlyPatchTaskOverrides: tCase.overrides}
			err := c.ValidateAndDefault()
			if tCase.expectedErr == "" {
				assert.NoError(t, err)
				return
			}
			assert.ErrorContains(t, err, tCase.expectedErr)
		})
	}
}
