package model

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/singleflight"
)

const (
	projectTranslationCacheTTL = 30 * time.Minute

	// defaultTranslationCacheBytesLimit bounds the cache when the admin setting is unset or
	// non-positive, so a missing config never leaves the cache unbounded. The budget is measured
	// against each entry's JSON-serialized size (see estimateProjectSize), which underestimates live
	// heap footprint, so the admin override should be set with headroom.
	defaultTranslationCacheBytesLimit = 256 * 1024 * 1024
)

// cachedTranslation is the cache value: the shared translated project plus the estimated byte cost
// recorded at insertion, so evictions can decrement the running byte total.
type cachedTranslation struct {
	project *Project
	size    int64
}

var (
	translationCacheMu        sync.Mutex
	translationCache          *expirable.LRU[string, cachedTranslation]
	translationGroupPtr       atomic.Pointer[singleflight.Group]
	translationCacheEvictions atomic.Int64

	// translationCacheWriteMu serializes each insert together with the eviction that brings the cache
	// back within budget, so the byte total stays consistent with the cache's contents across
	// concurrent inserts of different keys.
	translationCacheWriteMu sync.Mutex
	// translationCacheBytes is the running sum of the sizes of all live entries.
	translationCacheBytes atomic.Int64
	// translationCacheBytesLimit is the configured byte budget; a value <= 0 falls back to the default.
	translationCacheBytesLimit atomic.Int64

	// translationKeyHits counts cache hits per key so the hottest cached configs are identifiable for
	// tuning. Entries are removed when their key is evicted from the LRU, so it stays bounded by cache size.
	translationKeyHitsMu sync.Mutex
	translationKeyHits   = map[string]int64{}
)

func init() {
	translationGroupPtr.Store(&singleflight.Group{})
}

// getTranslationCache returns the process-wide LRU cache for translated projects, initializing it on
// first call. The LRU is created with no entry-count limit; the cache is bounded by the byte budget
// (see addToTranslationCache) and the TTL instead.
func getTranslationCache() *expirable.LRU[string, cachedTranslation] {
	translationCacheMu.Lock()
	defer translationCacheMu.Unlock()
	if translationCache == nil {
		translationCache = expirable.NewLRU[string, cachedTranslation](0, func(key string, v cachedTranslation) {
			translationCacheBytes.Add(-v.size)
			translationCacheEvictions.Add(1)
			translationKeyHitsMu.Lock()
			delete(translationKeyHits, key)
			translationKeyHitsMu.Unlock()
		}, projectTranslationCacheTTL)
	}
	return translationCache
}

// SetTranslationCacheBytesLimit sets the byte budget for the process-wide translation cache. A value
// <= 0 restores the built-in default, which also applies until this is first called.
func SetTranslationCacheBytesLimit(n int64) {
	translationCacheBytesLimit.Store(n)
}

func currentTranslationCacheBytesLimit() int64 {
	if n := translationCacheBytesLimit.Load(); n > 0 {
		return n
	}
	return defaultTranslationCacheBytesLimit
}

// estimateProjectSize approximates p's memory cost as the length of its JSON encoding. This
// underestimates live heap size but is proportional to it and, unlike the parser project, reflects
// matrix and variant expansion. It returns 0 if p can't be marshaled, so an unsizable entry doesn't
// block the cache.
func estimateProjectSize(p *Project) int64 {
	data, err := json.Marshal(p)
	if err != nil {
		return 0
	}
	return int64(len(data))
}

// addToTranslationCache inserts p under key and evicts least-recently-used entries until the running
// byte total is within the configured budget. Eviction is by recency only, with no size bias and no
// size-gated admission, so a large but actively used config stays resident while the coldest entries
// are dropped first. A single entry larger than the whole budget is kept rather than immediately
// evicted, so the budget can be exceeded transiently in that degenerate case.
func addToTranslationCache(key string, p *Project) {
	size := estimateProjectSize(p)
	c := getTranslationCache()

	translationCacheWriteMu.Lock()
	defer translationCacheWriteMu.Unlock()

	// Drop any prior (possibly TTL-expired but not-yet-swept) entry for this key so its bytes are
	// subtracted before accounting for the replacement.
	c.Remove(key)
	translationCacheBytes.Add(size)
	c.Add(key, cachedTranslation{project: p, size: size})

	limit := currentTranslationCacheBytesLimit()
	for translationCacheBytes.Load() > limit && c.Len() > 1 {
		c.RemoveOldest()
	}
}

// recordTranslationKeyHit increments the per-key hit counter for an LRU cache hit.
func recordTranslationKeyHit(key string) {
	translationKeyHitsMu.Lock()
	translationKeyHits[key]++
	translationKeyHitsMu.Unlock()
}

// hottestTranslationKey returns the cache key with the most hits and its hit count, or the zero
// values if nothing has been hit yet.
func hottestTranslationKey() (key string, hits int64) {
	translationKeyHitsMu.Lock()
	defer translationKeyHitsMu.Unlock()
	for k, v := range translationKeyHits {
		if v > hits {
			key, hits = k, v
		}
	}
	return key, hits
}

// translationCacheStats returns the LRU's current entry count, its cumulative eviction count
// (including TTL expirations, which expirable.LRU reports through the same callback), and the running
// byte total of the entries it's holding.
func translationCacheStats() (size int, evictions, bytes int64) {
	return getTranslationCache().Len(), translationCacheEvictions.Load(), translationCacheBytes.Load()
}

// getOrComputeTranslation coalesces concurrent calls via singleflight and, when cacheEnabled, also
// serves and stores results in a per-process LRU keyed by key, which must capture every input that
// determines the translation. The returned *Project is shared and must not be mutated.
//
// deduped reports whether this call was coalesced into another in-flight call for the same key.
// cacheHit reports an LRU hit that skipped compute entirely. The two are orthogonal.
func getOrComputeTranslation(key string, cacheEnabled bool, compute func() (*Project, error)) (project *Project, cacheHit bool, deduped bool, err error) {
	if cacheEnabled {
		if v, ok := getTranslationCache().Get(key); ok {
			recordTranslationKeyHit(key)
			return v.project, true, false, nil
		}
	}

	// compute can return a partial *Project alongside an error; package both into the value so
	// singleflight.Do doesn't drop the partial result via its own error return.
	type translationResult struct {
		project *Project
		err     error
	}
	v, sfErr, shared := translationGroupPtr.Load().Do(key, func() (any, error) {
		// Re-check after winning the singleflight slot: another goroutine may
		// have populated the cache while we were waiting.
		if cacheEnabled {
			if v, ok := getTranslationCache().Get(key); ok {
				recordTranslationKeyHit(key)
				return translationResult{project: v.project}, nil
			}
		}
		result, err := compute()
		if err != nil {
			return translationResult{project: result, err: err}, nil
		}
		if cacheEnabled {
			addToTranslationCache(key, result)
		}
		return translationResult{project: result}, nil
	})
	if sfErr != nil {
		return nil, false, shared, sfErr
	}
	tr := v.(translationResult)
	return tr.project, false, shared, tr.err
}

// versionTranslationKey is the cheap singleflight-only key for the version path when the cache is
// disabled. It must NOT be used as an LRU key; see getOrComputeTranslation.
func versionTranslationKey(versionID string, preGeneration bool) string {
	return fmt.Sprintf("v:%s:%v", versionID, preGeneration)
}

// fileTranslationKey is the cheap singleflight-only key for the file path when the cache is
// disabled. It must NOT be used as an LRU key; see getOrComputeTranslation.
func fileTranslationKey(projectID, revision string) string {
	return fmt.Sprintf("f:%s:%s", projectID, revision)
}

// contentTranslationKey keys a translation by a hash of the parser-project bytes plus identifier, so
// a changed parser project yields a new key and the LRU needs no invalidation.
func contentTranslationKey(contentsSHA, identifier string) string {
	return fmt.Sprintf("c:%s:%s", contentsSHA, identifier)
}

// translateAndCache translates pp with singleflight coalescing and, when cacheEnabled, the
// content-hash LRU; disabledKey is the singleflight-only key used when cacheEnabled is false. It
// returns an isolated clone because the underlying pointer is shared and callers mutate a project's
// top-level containers in place (see cloneForCacheReturn).
func translateAndCache(ctx context.Context, span trace.Span, pp *ParserProject, identifier, disabledKey string, cacheEnabled bool) (*Project, error) {
	key := disabledKey
	if cacheEnabled {
		sha, err := parserProjectContentSHA(pp)
		if err != nil {
			return nil, errors.Wrap(err, "computing parser project content hash")
		}
		key = contentTranslationKey(sha, identifier)
	}

	p, cacheHit, deduped, err := getOrComputeTranslation(key, cacheEnabled, func() (*Project, error) {
		return TranslateProject(ctx, pp)
	})
	setTranslationCacheSpanAttributes(span, cacheHit, deduped, cacheEnabled)
	// A partial *Project can come back alongside an error; still clone and return it.
	if p == nil {
		return nil, err
	}
	return p.cloneForCacheReturn(), err
}

// parserProjectContentSHA returns the hex-encoded SHA-256 of pp marshaled as JSON.
// Must be called after pp.Identifier is set so the identifier is included in the hash.
func parserProjectContentSHA(pp *ParserProject) (string, error) {
	data, err := json.Marshal(pp)
	if err != nil {
		return "", errors.Wrap(err, "marshaling parser project")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
