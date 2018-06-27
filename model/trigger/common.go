package trigger

import (
	"math"

	"github.com/evergreen-ci/evergreen/util"
)

const (
	selectorID        = "id"
	selectorObject    = "object"
	selectorProject   = "project"
	selectorOwner     = "owner"
	selectorRequester = "requester"
	selectorStatus    = "status"
	selectorInVersion = "in-version"
	selectorInBuild   = "in-build"

	triggerOutcome                = "outcome"
	triggerFailure                = "failure"
	triggerSuccess                = "success"
	triggerRegression             = "regression"
	triggerExceedsDuration        = "exceeds-duration"
	triggerRuntimeChangeByPercent = "runtime-change"
)

func runtimeExceedsThreshold(threshold, prevDuration, thisDuration float64) (bool, float64) {
	ratio := thisDuration / prevDuration
	if !util.IsFiniteNumericFloat(ratio) {
		return false, 0
	}
	percentChange := math.Abs(100*ratio - 100)
	return (percentChange >= threshold), percentChange
}
