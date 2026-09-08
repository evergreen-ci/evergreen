package evergreen

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"path"
	"strconv"
)

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

	// SourceCacheArchiveSuffix is the object key extension of every source cache artifact.
	SourceCacheArchiveSuffix = ".tgz"
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

// SourceCacheObjectKey returns the S3 object key holding one revision's artifact.
func SourceCacheObjectKey(owner, repo, namespace, branch, revision string, cloneDepth int, recurseSubmodules bool) string {
	key := SourceCacheKeyHash(owner, repo, namespace, branch, revision, cloneDepth, recurseSubmodules)
	return path.Join(SourceCacheNamespacePrefix(owner, repo, namespace), revision, key+SourceCacheArchiveSuffix)
}

// SourceCacheKeyHash returns the content hash folded into a revision's object key.
func SourceCacheKeyHash(owner, repo, namespace, branch, revision string, cloneDepth int, recurseSubmodules bool) string {
	// A PR artifact is pinned to the PR head, so the branch would only fragment its key.
	if namespace == SourceCachePRNamespace {
		branch = ""
	}
	expansions := []string{namespace, owner, repo, branch, revision, strconv.Itoa(cloneDepth), strconv.FormatBool(recurseSubmodules)}
	return sourceCacheHash(expansions)
}

// sourceCacheHash hashes the key inputs the way the agent's computeCacheKey hashes
// an expansion list with no key files and preserveSymlinks set.
func sourceCacheHash(expansions []string) string {
	h := sha256.New()
	writeUint64 := func(n int) {
		var buf [8]byte
		binary.BigEndian.PutUint64(buf[:], uint64(n))
		h.Write(buf[:])
	}
	writeUint64(0)
	writeUint64(len(expansions))
	for _, value := range expansions {
		writeUint64(len(value))
		h.Write([]byte(value))
	}
	h.Write([]byte{1})
	return hex.EncodeToString(h.Sum(nil))
}
