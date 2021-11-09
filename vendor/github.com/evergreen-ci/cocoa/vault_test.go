package cocoa

import (
	"testing"

	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNamedSecret(t *testing.T) {
	t.Run("NewNamedSecret", func(t *testing.T) {
		s := NewNamedSecret()
		require.NotZero(t, s)
		assert.Zero(t, *s)
	})
	t.Run("SetName", func(t *testing.T) {
		name := "name"
		s := NewNamedSecret().SetName(name)
		assert.Equal(t, name, utility.FromStringPtr(s.Name))
	})
	t.Run("SetValue", func(t *testing.T) {
		val := "value"
		s := NewNamedSecret().SetValue(val)
		assert.Equal(t, val, utility.FromStringPtr(s.Value))
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("EmptyIsInvalid", func(t *testing.T) {
			s := NewNamedSecret()
			assert.Error(t, s.Validate())
		})
		t.Run("NameAndValueIsValid", func(t *testing.T) {
			s := NewNamedSecret().SetName("name").SetValue("value")
			assert.NoError(t, s.Validate())
		})
		t.Run("MissingNameIsInvalid", func(t *testing.T) {
			s := NewNamedSecret().SetValue("value")
			assert.Error(t, s.Validate())
		})
		t.Run("EmptyNameIsInvalid", func(t *testing.T) {
			s := NewNamedSecret().SetName("").SetValue("value")
			assert.Error(t, s.Validate())
		})
		t.Run("MissingValueIsInvalid", func(t *testing.T) {
			s := NewNamedSecret().SetName("name")
			assert.Error(t, s.Validate())
		})
	})
}
