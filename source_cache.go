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

	// SourceCacheExternalIDPrefix marks the source cache's AssumeRole calls. The
	// role's trust policy requires it, so a task cannot assume the role through the
	// generic route, whose external ID starts with the caller-supplied project ID.
	// The colon is load-bearing: it is legal in an external ID but not a project ID,
	// so no project can forge the prefix.
	SourceCacheExternalIDPrefix = "source-cache:"
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
