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
				buildVariantRegexp:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, criteria)
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
				buildVariantRegexp:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 1,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, criteria)
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, &client.Mock{})
		})
	}
}

func TestLastRevisionCheckVersions(t *testing.T) {
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
				buildVariantRegexp:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 0.5,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, criteria)
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
				buildVariantRegexp:   []regexp.Regexp{*regexp.MustCompile("bv1")},
				minSuccessProportion: 1,
			}

			passesCriteria, err := checkBuildsPassCriteria(t.Context(), c, builds, criteria)
			require.NoError(t, err)
			assert.False(t, passesCriteria)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, &client.Mock{})
		})
	}
}
