package operations

import (
	"regexp"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastRevisionCheckBuilds(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, c *client.Mock){
		"PassesCriteriaWithBuildSuccessRateAboveThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"DoesNotPassCriteriaWithBuildSuccessRateBelowThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 1,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
		"PassesCriteriaWithNoMatchingBuildVariants": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("nonexistent")},
				minSuccessProportion: 0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"PassesCriteriaWithMatchingBuildVariantDisplayNameWhenSuccessRateIsAboveThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					DisplayName:  utility.ToStringPtr("Build variant 1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:                        "test_project",
				buildVariantDisplayNameRegexps: []regexp.Regexp{*regexp.MustCompile("variant 1")},
				minSuccessProportion:           0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"DoesNotPassCriteriaWithMatchingBuildVariantDisplayNameWhenSuccessRateIsBelowThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					DisplayName:  utility.ToStringPtr("Build variant 1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:                        "test_project",
				buildVariantDisplayNameRegexps: []regexp.Regexp{*regexp.MustCompile("variant 1")},
				minSuccessProportion:           1,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
		"PassesCriteriaWithNoMatchingBuildVariantDisplayNames": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					DisplayName:  utility.ToStringPtr("Build variant 1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:                        "test_project",
				buildVariantDisplayNameRegexps: []regexp.Regexp{*regexp.MustCompile("nonexistent")},
				minSuccessProportion:           0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"PassesCriteriaWithBuildFinishedRateAboveThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:               "test_project",
				buildVariantRegexps:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minFinishedProportion: 0.9,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"DoesNotPassCriteriaWithBuildFinishedRateBelowThreshold": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskStarted),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t3"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskUndispatched),
				},
			}
			criteria := lastRevisionCriteria{
				project:               "test_project",
				buildVariantRegexps:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minFinishedProportion: 0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
		"PassesCriteriaWithRequiredSuccessfulTaskInBuild": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					DisplayName:  utility.ToStringPtr("Task 1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:             "test_project",
				buildVariantRegexps: []regexp.Regexp{*regexp.MustCompile("bv1")},
				successfulTasks:     []string{"Task 1"},
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"PassesCriteriaWithRequiredSuccessfulTaskNotInBuild": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:             "test_project",
				buildVariantRegexps: []regexp.Regexp{*regexp.MustCompile("bv1")},
				successfulTasks:     []string{"nonexistent"},
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"DoesNotPassCriteriaWithRequiredSuccessfulTaskFailingInBuild": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					DisplayName:  utility.ToStringPtr("Task 1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:             "test_project",
				buildVariantRegexps: []regexp.Regexp{*regexp.MustCompile("bv1")},
				successfulTasks:     []string{"Task 1"},
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
		"PassesCriteriaWithBuildSuccessRateAboveThresholdCountingKnownIssuesAsSuccesses": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:             utility.ToStringPtr("t1"),
					BuildId:        utility.ToStringPtr("b1"),
					BuildVariant:   utility.ToStringPtr("bv1"),
					Status:         utility.ToStringPtr(evergreen.TaskFailed),
					HasAnnotations: true,
				},
				{
					Id:           utility.ToStringPtr("t2"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:               "test_project",
				buildVariantRegexps:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion:  1,
				knownIssuesAreSuccess: true,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
		"PassesCriteriaWithRequiredSuccessfulTaskInBuildCountingKnownIssuesAsSuccesses": func(t *testing.T, c *client.Mock) {
			builds := []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:             utility.ToStringPtr("t1"),
					DisplayName:    utility.ToStringPtr("Task 1"),
					BuildId:        utility.ToStringPtr("b1"),
					BuildVariant:   utility.ToStringPtr("bv1"),
					Status:         utility.ToStringPtr(evergreen.TaskFailed),
					HasAnnotations: true,
				},
			}
			criteria := lastRevisionCriteria{
				project:               "test_project",
				buildVariantRegexps:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				successfulTasks:       []string{"Task 1"},
				knownIssuesAreSuccess: true,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			assert.True(t, passesCriteria)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, &client.Mock{})
		})
	}
}

func TestLastRevisionCheckVersions(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, c *client.Mock){
		"ReturnsVersionWithMatchingBuildSuccessRateAboveThreshold": func(t *testing.T, c *client.Mock) {
			versions := []model.APIVersion{
				{
					Id:      utility.ToStringPtr("v1"),
					Project: utility.ToStringPtr("test_project"),
				},
			}
			c.GetBuildsForVersionResult = []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					Version:      utility.ToStringPtr("v1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					Version:      utility.ToStringPtr("v1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					Version:      utility.ToStringPtr("v1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 0.5,
			}

			v, err := findLatestMatchingVersion(t.Context(), c, versions, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			require.NotNil(t, v)
			assert.Equal(t, "v1", utility.FromStringPtr(v.Id))
		},
		"DoesNotReturnVersionWithBuildSuccessRateBelowThreshold": func(t *testing.T, c *client.Mock) {
			versions := []model.APIVersion{
				{
					Id:      utility.ToStringPtr("v1"),
					Project: utility.ToStringPtr("test_project"),
				},
			}
			c.GetBuildsForVersionResult = []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					Version:      utility.ToStringPtr("v1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					Version:      utility.ToStringPtr("v1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
				{
					Id:           utility.ToStringPtr("t2"),
					Version:      utility.ToStringPtr("v1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskSucceeded),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 1,
			}

			v, err := findLatestMatchingVersion(t.Context(), c, versions, []lastRevisionCriteria{criteria})
			assert.NoError(t, err)
			assert.Nil(t, v)
		},
		"ReturnsVersionWithNoMatchingBuildVariants": func(t *testing.T, c *client.Mock) {
			versions := []model.APIVersion{
				{
					Id:      utility.ToStringPtr("v1"),
					Project: utility.ToStringPtr("test_project"),
				},
			}
			c.GetBuildsForVersionResult = []model.APIBuild{
				{
					Id:           utility.ToStringPtr("b1"),
					Version:      utility.ToStringPtr("v1"),
					BuildVariant: utility.ToStringPtr("bv1"),
				},
			}
			c.GetTasksForBuildResult = []model.APITask{
				{
					Id:           utility.ToStringPtr("t1"),
					Version:      utility.ToStringPtr("v1"),
					BuildId:      utility.ToStringPtr("b1"),
					BuildVariant: utility.ToStringPtr("bv1"),
					Status:       utility.ToStringPtr(evergreen.TaskFailed),
				},
			}
			criteria := lastRevisionCriteria{
				project:              "test_project",
				buildVariantRegexps:  []regexp.Regexp{*regexp.MustCompile("nonexistent")},
				minSuccessProportion: 0.5,
			}

			v, err := findLatestMatchingVersion(t.Context(), c, versions, []lastRevisionCriteria{criteria})
			require.NoError(t, err)
			require.NotNil(t, v)
			assert.Equal(t, "v1", utility.FromStringPtr(v.Id))
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, &client.Mock{})
		})
	}
}

func TestLastRevisionCriteriaReuse(t *testing.T) {
	criteria1 := reusableLastRevisionCriteria{
		BVRegexps:             []string{"bv1"},
		MinSuccessProportion:  0.5,
		MinFinishedProportion: 0.9,
		SuccessfulTasks:       []string{"task1", "task2"},
	}
	criteria2 := reusableLastRevisionCriteria{
		BVDisplayRegexps: []string{"Build Variant 2"},
		SuccessfulTasks:  []string{"task3"},
	}
	conf := &ClientSettings{
		LastRevisionCriteriaGroups: []lastRevisionCriteriaGroup{
			{
				Name:     "group1",
				Criteria: []reusableLastRevisionCriteria{criteria1, criteria2},
			},
		},
	}
	t.Run("ReturnsExistingCriteriaGroup", func(t *testing.T) {
		allCriteria, err := getLastRevisionCriteria(conf, "group1", "project", true)
		assert.NoError(t, err)
		require.Len(t, allCriteria, 2)
		for _, c := range allCriteria {
			assert.Equal(t, "project", c.project)
			assert.True(t, c.knownIssuesAreSuccess)
		}

		assert.Len(t, allCriteria[0].buildVariantRegexps, len(criteria1.BVRegexps))
		assert.Len(t, allCriteria[0].buildVariantDisplayNameRegexps, len(criteria1.BVDisplayRegexps))
		assert.Equal(t, criteria1.MinSuccessProportion, allCriteria[0].minSuccessProportion)
		assert.Equal(t, criteria1.MinFinishedProportion, allCriteria[0].minFinishedProportion)
		assert.Len(t, allCriteria[0].successfulTasks, len(criteria1.SuccessfulTasks))

		assert.Len(t, allCriteria[1].buildVariantRegexps, len(criteria2.BVRegexps))
		assert.Len(t, allCriteria[1].buildVariantDisplayNameRegexps, len(criteria2.BVDisplayRegexps))
		assert.Zero(t, allCriteria[1].minSuccessProportion, criteria2.MinSuccessProportion)
		assert.Zero(t, allCriteria[1].minFinishedProportion, criteria2.MinFinishedProportion)
		assert.Len(t, allCriteria[1].successfulTasks, len(criteria2.SuccessfulTasks))
	})
	t.Run("ErrorsForNonexistentCriteriaGroup", func(t *testing.T) {
		_, err := getLastRevisionCriteria(conf, "nonexistent", "project", true)
		assert.Error(t, err)
	})
}
