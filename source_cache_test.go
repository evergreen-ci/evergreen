package evergreen

import (
	"crypto/sha256"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceCacheExternalIDCannotBeForgedByAProjectID(t *testing.T) {
	// Kept in sync with projectIDRegexp in rest/data/project.go, which importing
	// here would be a cycle.
	projectIDRegexp := regexp.MustCompile(`^[0-9a-zA-Z-._~\(\) ]*$`)
	// The ExternalId pattern from the STS AssumeRole API.
	externalIDRegexp := regexp.MustCompile(`^[\w+=,.@:/-]*$`)

	require.Regexp(t, externalIDRegexp, SourceCacheExternalID, "the external ID must be a legal STS external ID")

	// The generic route's external ID is "<project ID>-<requester>", so a project
	// that could spell this external ID could assume the role through that route.
	assert.NotRegexp(t, projectIDRegexp, SourceCacheExternalID, "no project ID may be able to spell the external ID")
}

func TestSourceCacheKeyHashAndObjectKey(t *testing.T) {
	const (
		owner     = "some-org"
		repo      = "some-repo"
		namespace = SourceCacheBaseNamespace
		revision  = "abc123"
	)
	key := SourceCacheKeyHash(owner, repo, namespace, "main", revision, 1000, true)
	require.Len(t, key, sha256.Size*2)
	// The object key layout is the contract external consumers compute against.
	assert.Equal(t, "source_cache/v1/some-org/some-repo/base/abc123/"+key+".tgz",
		SourceCacheObjectKey(owner, repo, namespace, "main", revision, 1000, true))
	// Clone shape and revision are part of the key.
	assert.NotEqual(t, key, SourceCacheKeyHash(owner, repo, namespace, "main", revision, 1, true))
	assert.NotEqual(t, key, SourceCacheKeyHash(owner, repo, namespace, "main", revision, 1000, false))
	assert.NotEqual(t, key, SourceCacheKeyHash(owner, repo, namespace, "main", "def456", 1000, true))
}

func TestSourceCachePRNamespaceKeyIgnoresTheBranch(t *testing.T) {
	// A PR artifact is pinned to the PR head, so the branch cannot fragment its key.
	assert.Equal(t,
		SourceCacheKeyHash("org", "repo", SourceCachePRNamespace, "release-v1", "abc123", 1, false),
		SourceCacheKeyHash("org", "repo", SourceCachePRNamespace, "main", "abc123", 1, false))
	// Base namespace artifacts are keyed on the branch.
	assert.NotEqual(t,
		SourceCacheKeyHash("org", "repo", SourceCacheBaseNamespace, "release-v1", "abc123", 1, false),
		SourceCacheKeyHash("org", "repo", SourceCacheBaseNamespace, "main", "abc123", 1, false))
}
