package management

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterValidation(t *testing.T) {
	t.Run("Status", func(t *testing.T) {
		for _, f := range []StatusFilter{Pending, InProgress, Stale, Completed, Retrying, All} {
			assert.Nil(t, f.Validate())
		}
	})

	t.Run("InvalidValues", func(t *testing.T) {
		for _, f := range []string{"", "foo", "bleh", "0"} {
			assert.Error(t, StatusFilter(f).Validate())
		}
	})
}
