package parameterstore

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// This tests basic functionality in the Parameter Manager.
// It's not possible to integration test directly with an SSM client in this
// package because tests do not have a real Parameter Store instance to use.
// Furthermore, using the fake Parameter Store client implementation in this
// package would produce a circular package dependency.
func TestParameterManager(t *testing.T) {
	pm := NewParameterManager("prefix", false, nil, nil)
	t.Run("GetPrefixedName", func(t *testing.T) {
		t.Run("PrefixesBasenames", func(t *testing.T) {
			assert.Equal(t, "/prefix/basename", pm.getPrefixedName("basename"))
		})
		t.Run("PrefixesPartialFullNames", func(t *testing.T) {
			assert.Equal(t, "/prefix/path/to/basename", pm.getPrefixedName("/path/to/basename"))
		})
		t.Run("NoopsForAlreadyPrefixedName", func(t *testing.T) {
			assert.Equal(t, "/prefix/path/to/basename", pm.getPrefixedName("/prefix/path/to/basename"))
		})
	})
	t.Run("GetBasename", func(t *testing.T) {
		t.Run("ParsesBasenameFromFullNameWithPrefix", func(t *testing.T) {
			fullName := pm.getPrefixedName("basename")
			assert.Equal(t, "/prefix/basename", fullName)
			assert.Equal(t, "basename", pm.getBasename(fullName))
		})
		t.Run("ParsesBasenameFromFullNameWithoutPrefix", func(t *testing.T) {
			fullName := "/some/other/path/basename"
			assert.Equal(t, "basename", pm.getBasename(fullName))
		})
		t.Run("ReturnsUnmodifiedBasenameThatIsAlreadyParsed", func(t *testing.T) {
			assert.Equal(t, "basename", pm.getBasename("basename"))
		})
	})
}
