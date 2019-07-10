package trigger

import (
	"math"

	"github.com/evergreen-ci/evergreen/util"
)

func runtimeExceedsThreshold(threshold, prevDuration, thisDuration float64) (bool, float64) {
	ratio := thisDuration / prevDuration
	if !util.IsFiniteNumericFloat(ratio) {
		return false, 0
	}
	percentChange := math.Abs(100*ratio - 100)
	return (percentChange >= threshold), percentChange
}
