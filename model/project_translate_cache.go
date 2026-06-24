package model

import (
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
	"golang.org/x/sync/singleflight"
)

const (
	projectTranslationCacheSize = 512
	projectTranslationCacheTTL  = 30 * time.Minute
	translationCacheEnabled     = false
)

var (
	translationCacheMu  sync.Mutex
	translationCache    *expirable.LRU[string, *Project]
	translationGroupPtr atomic.Pointer[singleflight.Group]
)

func init() {
	translationGroupPtr.Store(&singleflight.Group{})
}

func getTranslationCache() *expirable.LRU[string, *Project] {
	translationCacheMu.Lock()
	defer translationCacheMu.Unlock()
	if translationCache == nil {
		translationCache = expirable.NewLRU[string, *Project](projectTranslationCacheSize, nil, projectTranslationCacheTTL)
	}
	return translationCache
}

// getOrComputeTranslation coalesces concurrent calls via singleflight and, when cacheEnabled, also
// serves and stores results in a per-process LRU. When cacheEnabled, key must capture every input
// that determines the translation. The cached *Project is shared and must not be mutated.
func getOrComputeTranslation(key string, cacheEnabled bool, compute func() (*Project, error)) (*Project, bool, error) {
	if cacheEnabled {
		if v, ok := getTranslationCache().Get(key); ok {
			return v, true, nil
		}
	}

	v, err, _ := translationGroupPtr.Load().Do(key, func() (any, error) {
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
		return nil, false, err
	}
	return v.(*Project), false, nil
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
