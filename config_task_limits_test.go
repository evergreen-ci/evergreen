package evergreen

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHourlyPatchTaskLimitForProject(t *testing.T) {
	for tName, tCase := range map[string]struct {
		config                  TaskLimitsConfig
		projectID               string
		repoRefID               string
		expectedLimit           int
		expectedProjectOrRepoID string
	}{
		"NoOverridesUsesGeneralLimit": {
			config: TaskLimitsConfig{
				MaxHourlyPatchTasks: 15000,
			},
			projectID:               "project",
			expectedLimit:           15000,
			expectedProjectOrRepoID: "",
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
			projectID:               "project",
			expectedLimit:           40000,
			expectedProjectOrRepoID: "project",
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
			projectID:               "project",
			expectedLimit:           15000,
			expectedProjectOrRepoID: "",
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
			projectID:               "project",
			repoRefID:               "repo",
			expectedLimit:           40000,
			expectedProjectOrRepoID: "repo",
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
			projectID:               "project",
			repoRefID:               "other_repo",
			expectedLimit:           15000,
			expectedProjectOrRepoID: "",
		},
		"BranchProjectOverrideTakesPrecedenceOverRepoOverrideWhenBothCouldApply": {
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
			projectID:               "project",
			repoRefID:               "repo",
			expectedLimit:           20000,
			expectedProjectOrRepoID: "project",
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
			projectID:               "project",
			expectedLimit:           40000,
			expectedProjectOrRepoID: "project",
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
			projectID:               "project",
			expectedLimit:           0,
			expectedProjectOrRepoID: "",
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
			projectID:               "project",
			expectedLimit:           100,
			expectedProjectOrRepoID: "project",
		},
	} {
		t.Run(tName, func(t *testing.T) {
			limit, scopeID := tCase.config.HourlyPatchTaskLimitForProject(tCase.projectID, tCase.repoRefID)
			assert.Equal(t, tCase.expectedLimit, limit)
			assert.Equal(t, tCase.expectedProjectOrRepoID, scopeID)
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
		"OverrideWithoutAProjectOrRepoIDShouldError": {
			overrides: []HourlyPatchTaskOverride{
				{
					MaxHourlyPatchTasks: 40000,
				},
			},
			expectedErr: "must set a project/repo ID",
		},
		"MultipleUniqueOverridesShouldSucceed": {
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
