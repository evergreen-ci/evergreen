package parameterstore

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

// This tests basic functionality in the Parameter Manager.
// It's not possible to integration test directly with an SSM client in this
// package because tests do not have a real Parameter Store instance to use.
// Furthermore, using the fake Parameter Store client implementation in this
// package would produce a circular package dependency.
func TestParameterManager(t *testing.T) {
	for prefixTestCase, prefix := range map[string]string{
		// Check resiliency against various possible slashes in the path prefix.
		"PathPrefixWithoutLeadingOrTrailingSlash": "prefix",
		"PathPrefixWithLeadingSlash":              "/prefix",
		"PathPrefixWithTrailingSlash":             "prefix/",
		"PathPrefixWithLeadingAndTrailingSlash":   "/prefix/",
	} {
		t.Run(prefixTestCase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := ParameterManagerOptions{
				PathPrefix: prefix,
				DB:         &mongo.Database{},
			}
			require.NoError(t, opts.Validate(ctx))
			pm := &ParameterManager{
				pathPrefix: opts.PathPrefix,
			}
			t.Run("GetPrefixedName", func(t *testing.T) {
				t.Run("PrefixesBasename", func(t *testing.T) {
					assert.Equal(t, "/prefix/basename", pm.getPrefixedName("basename"))
				})
				t.Run("PrefixesPartialName", func(t *testing.T) {
					assert.Equal(t, "/prefix/path/to/basename", pm.getPrefixedName("/path/to/basename"))
				})
				t.Run("PrefixesPartialNamesWithoutLeadingSlash", func(t *testing.T) {
					assert.Equal(t, "/prefix/path/to/basename", pm.getPrefixedName("path/to/basename"))
				})
				t.Run("NoopsForAlreadyPrefixedName", func(t *testing.T) {
					assert.Equal(t, "/prefix/path/to/basename", pm.getPrefixedName("/prefix/path/to/basename"))
				})
			})

			t.Run("GetBasename", func(t *testing.T) {
				t.Run("ParsesBasenameFromFullNameWithPrefix", func(t *testing.T) {
					fullName := pm.getPrefixedName("basename")
					assert.Equal(t, "/prefix/basename", fullName)
					assert.Equal(t, "basename", getBasename(fullName))
				})
				t.Run("ParsesBasenameFromFullNameWithoutPrefix", func(t *testing.T) {
					fullName := "/some/other/path/basename"
					assert.Equal(t, "basename", getBasename(fullName))
				})
				t.Run("ReturnsUnmodifiedBasenameThatIsAlreadyParsed", func(t *testing.T) {
					assert.Equal(t, "basename", getBasename("basename"))
				})
				t.Run("ParsesBasenameWithoutLeadingSlash", func(t *testing.T) {
					fullName := "/basename"
					assert.Equal(t, "basename", getBasename(fullName))
				})
			})
		})
	}
}
