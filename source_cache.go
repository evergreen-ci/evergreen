package evergreen

import "path"

const (
	// SourceCachePrefix is the top-level S3 key prefix for source cache artifacts.
	SourceCachePrefix = "source_cache/v1"

	// SourceCacheBaseNamespace holds artifacts produced from a repo's own revisions.
	SourceCacheBaseNamespace = "base"

	// SourceCachePRNamespace holds artifacts produced from unreviewed code, so the
	// credentials handed to a PR task can deny writes to the base namespace.
	SourceCachePRNamespace = "pr"

	// SourceCacheExternalID is the fixed external ID for source cache role assumptions.
	SourceCacheExternalID = "source-cache:"
)

// SourceCacheRepoPrefix returns the key prefix holding a repo's artifacts across
// every namespace.
func SourceCacheRepoPrefix(owner, repo string) string {
	return path.Join(SourceCachePrefix, owner, repo)
}

// SourceCacheNamespacePrefix returns the key prefix holding a repo's artifacts for
// one namespace.
func SourceCacheNamespacePrefix(owner, repo, namespace string) string {
	return path.Join(SourceCacheRepoPrefix(owner, repo), namespace)
}
