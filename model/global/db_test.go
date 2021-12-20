package global

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

func TestGetNewBuildVariantBuildNumber(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"ReturnsIncrementedCounterForNewBuildVariant": func(t *testing.T) {
			num, err := GetNewBuildVariantBuildNumber("bv")
			require.NoError(t, err)
			assert.EqualValues(t, 1, num)
		},
		"IncrementsCounterForBuildVariant": func(t *testing.T) {
			for i := 0; i < 10; i++ {
				num, err := GetNewBuildVariantBuildNumber("bv")
				require.NoError(t, err)
				assert.EqualValues(t, i+1, num)
			}
		},
		"TracksSeparateCountersPerBuildVariant": func(t *testing.T) {
			num1, err := GetNewBuildVariantBuildNumber("bv1")
			require.NoError(t, err)
			assert.EqualValues(t, 1, num1)
			num1, err = GetNewBuildVariantBuildNumber("bv1")
			require.NoError(t, err)
			assert.EqualValues(t, 2, num1)

			num2, err := GetNewBuildVariantBuildNumber("bv2")
			require.NoError(t, err)
			assert.EqualValues(t, 1, num2)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(Collection))
			defer func() {
				assert.NoError(t, db.Clear(Collection))
			}()
			tCase(t)
		})
	}
}
