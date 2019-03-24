package jasper

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilters(t *testing.T) {
	t.Run("ConstantsValidate", func(t *testing.T) {
		for _, f := range []Filter{Running, Terminated, All, Failed, Successful} {
			assert.NoError(t, f.Validate())
		}
	})
	t.Run("ConstantEquavalentsValidate", func(t *testing.T) {
		for _, f := range []Filter{"running", "terminated", "all", "failed", "successful"} {
			assert.NoError(t, f.Validate())
		}
	})
	t.Run("OtherValuesDoNotValidate", func(t *testing.T) {
		for _, f := range []Filter{"", "foo", "terminate", "terminator", "fail"} {
			assert.Error(t, f.Validate())
		}
	})
}
