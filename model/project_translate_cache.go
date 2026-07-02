package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"github.com/pkg/errors"
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
)

func init() {
	translationGroupPtr.Store(&singleflight.Group{})
}

// getTranslationCache returns the process-wide LRU cache for translated projects, initializing it on first call.
func getTranslationCache() *expirable.LRU[string, *Project] {
	translationCacheMu.Lock()
	defer translationCacheMu.Unlock()
	if translationCache == nil {
		translationCache = expirable.NewLRU[string, *Project](projectTranslationCacheSize, func(_ string, _ *Project) {
			translationCacheEvictions.Add(1)
		}, projectTranslationCacheTTL)
	}
	return translationCache
}

// translationCacheStats returns the LRU's current entry count and its cumulative eviction count
// (including TTL expirations, which expirable.LRU reports through the same callback).
func translationCacheStats() (size int, evictions int64) {
	return getTranslationCache().Len(), translationCacheEvictions.Load()
}

// getOrComputeTranslation coalesces concurrent calls via singleflight and, when cacheEnabled, also
// serves and stores results in a per-process LRU. When cacheEnabled, key must capture every input
// that determines the translation. The cached *Project is shared and must not be mutated.
//
// deduped reports whether this call's singleflight request was coalesced into another in-flight
// call for the same key; it is meaningful whether or not the cache is enabled. cacheHit reports
// whether the result was served from the LRU without calling compute at all, which is only
// possible when the cache is enabled. The two are orthogonal.
func getOrComputeTranslation(key string, cacheEnabled bool, compute func() (*Project, error)) (project *Project, cacheHit bool, deduped bool, err error) {
	if cacheEnabled {
		if v, ok := getTranslationCache().Get(key); ok {
			return v, true, false, nil
		}
	}

	v, err, shared := translationGroupPtr.Load().Do(key, func() (any, error) {
		// Re-check after winning the singleflight slot: another goroutine may
		// have populated the cache while we were waiting.
		if cacheEnabled {
			if v, ok := getTranslationCache().Get(key); ok {
				return v, nil
			}
		}
		result, err := compute()
		if err != nil {
			return nil, err
		}
		if cacheEnabled {
			getTranslationCache().Add(key, result)
		}
		return result, nil
	})
	if err != nil {
		return nil, false, shared, err
	}
	return v.(*Project), false, shared, nil
}

// versionTranslationKey is the cheap singleflight key for the cache-disabled path. It must NOT be
// used as an LRU key; see getOrComputeTranslation.
func versionTranslationKey(versionID string, preGeneration bool) string {
	return fmt.Sprintf("v:%s:%v", versionID, preGeneration)
}

// contentTranslationKey keys a translation by a hash of the parser-project bytes plus identifier, so
// a changed parser project yields a new key and the LRU needs no invalidation.
func contentTranslationKey(contentsSHA, identifier string) string {
	return fmt.Sprintf("c:%s:%s", contentsSHA, identifier)
}

// fileTranslationKey is the self-invalidating LRU key for the LoadProjectInto / GetProjectFromFile path.
func fileTranslationKey(projectID, revision, contentsSHA string) string {
	return fmt.Sprintf("f:%s:%s:%s", projectID, revision, contentsSHA)
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
