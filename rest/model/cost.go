package model

import "github.com/evergreen-ci/evergreen"

// shouldHideCostForProject reports whether cost fields should be suppressed in API responses for the
// given project ID.
func shouldHideCostForProject(projectID string) bool {
	if projectID == "" {
		return false
	}
	env := evergreen.GetEnvironment()
	if env == nil {
		return false
	}
	settings := env.Settings()
	if settings == nil {
		return false
	}
	return settings.Cost.ShouldHideCost(projectID)
}
