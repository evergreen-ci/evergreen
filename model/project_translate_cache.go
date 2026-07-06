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
	projectTranslationCacheSize = 512
	projectTranslationCacheTTL  = 30 * time.Minute
)

var (
	translationCacheMu        sync.Mutex
	translationCache          *expirable.LRU[string, *Project]
	translationGroupPtr       atomic.Pointer[singleflight.Group]
	translationCacheEvictions atomic.Int64

	// translationKeyHits counts cache hits per key so the hottest cached configs are identifiable for
	// tuning. Entries are removed when their key is evicted from the LRU, so it stays bounded by cache size.
	translationKeyHitsMu sync.Mutex
	translationKeyHits   = map[string]int64{}
)

func init() {
	translationGroupPtr.Store(&singleflight.Group{})
}

// getTranslationCache returns the process-wide LRU cache for translated projects, initializing it on first call.
func getTranslationCache() *expirable.LRU[string, *Project] {
	translationCacheMu.Lock()
	defer translationCacheMu.Unlock()
	if translationCache == nil {
		translationCache = expirable.NewLRU[string, *Project](projectTranslationCacheSize, func(key string, _ *Project) {
			translationCacheEvictions.Add(1)
			translationKeyHitsMu.Lock()
			delete(translationKeyHits, key)
			translationKeyHitsMu.Unlock()
		}, projectTranslationCacheTTL)
	}
	return translationCache
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

// translationCacheStats returns the LRU's current entry count and its cumulative eviction count
// (including TTL expirations, which expirable.LRU reports through the same callback).
func translationCacheStats() (size int, evictions int64) {
	return getTranslationCache().Len(), translationCacheEvictions.Load()
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
			return v, true, false, nil
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
				return translationResult{project: v}, nil
			}
		}
		result, err := compute()
		if err != nil {
			return translationResult{project: result, err: err}, nil
		}
		if cacheEnabled {
			getTranslationCache().Add(key, result)
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
