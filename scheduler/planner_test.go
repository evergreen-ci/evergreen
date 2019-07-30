package scheduler

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPlanner(t *testing.T) {
	t.Run("Caches", func(t *testing.T) {
		t.Run("StringSet", func(t *testing.T) {
			t.Run("ZeroValue", func(t *testing.T) {
				assert.Len(t, StringSet{}, 0)
				assert.NotNil(t, StringSet{})
			})
			t.Run("CheckNonExistant", func(t *testing.T) {
				set := StringSet{}
				assert.False(t, set.Check("foo"))
			})
			t.Run("CheckExisting", func(t *testing.T) {
				set := StringSet{}
				set.Add("foo")
				assert.Len(t, set, 1)
				assert.True(t, set.Check("foo"))
			})
			t.Run("VisitNewKey", func(t *testing.T) {
				set := StringSet{}
				assert.False(t, set.Check("foo"))
				assert.False(t, set.Visit("foo"))
				assert.True(t, set.Check("foo"))
			})
			t.Run("VistExistingKey", func(t *testing.T) {
				set := StringSet{}
				assert.Len(t, set, 0)
				set.Add("foo")
				assert.Len(t, set, 1)
				assert.True(t, set.Visit("foo"))
				assert.Len(t, set, 1)
			})
		})
		t.Run("UnitCache", func(t *testing.T) {

		})
		t.Run("Unit", func(t *testing.T) {

		})
	})
	t.Run("GenerateTaskPlan", func(t *testing.T) {
		t.Run("ExportDeduplicates", func(t *testing.T) {

		})
		t.Run("VersionsGrouped", func(t *testing.T) {

		})
		t.Run("DependenciesGrouped", func(t *testing.T) {

		})
		t.Run("TaskGroupsGrouped", func(t *testing.T) {

		})
	})
	t.Run("Sorting", func(t *testing.T) {
		t.Run("UnitInternal", func(t *testing.T) {

		})
		t.Run("UnitExternal", func(t *testing.T) {

		})
	})
}
