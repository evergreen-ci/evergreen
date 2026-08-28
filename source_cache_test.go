package evergreen

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceCacheExternalIDPrefixCannotBeForgedByAProjectID(t *testing.T) {
	// Kept in sync with projectIDRegexp in rest/data/project.go, which importing
	// here would be a cycle.
	projectIDRegexp := regexp.MustCompile(`^[0-9a-zA-Z-._~\(\) ]*$`)
	// The ExternalId pattern from the STS AssumeRole API.
	externalIDRegexp := regexp.MustCompile(`^[\w+=,.@:/-]*$`)

	require.Regexp(t, externalIDRegexp, SourceCacheExternalIDPrefix, "the prefix must be a legal STS external ID")

	// The generic route's external ID is "<project ID>-<requester>", so a project
	// that could spell this prefix could assume the role through that route.
	assert.NotRegexp(t, projectIDRegexp, SourceCacheExternalIDPrefix, "no project ID may be able to spell the prefix")
}
