package parameterstore

import (
	"sync"
	"time"

	"github.com/evergreen-ci/utility"
)

// parameterCache is a thread-safe cache for parameter values.
type parameterCache struct {
	cache map[string]cachedParameter
	mu    sync.RWMutex
}

func newParameterCache() *parameterCache {
	return &parameterCache{
		cache: map[string]cachedParameter{},
	}
}

type cachedParameter struct {
	// name is the full name of the parameter.
	name string
	// value is the plaintext value of the parameter.
	value string
	// lastUpdated is the time the parameter was last updated.
	lastUpdated time.Time
}

func newCachedParameter(name, value string, lastUpdated time.Time) cachedParameter {
	return cachedParameter{
		name:        name,
		value:       value,
		lastUpdated: lastUpdated,
	}
}

func (cp *cachedParameter) export() Parameter {
	return Parameter{
		Name:     cp.name,
		Basename: getBasename(cp.name),
		Value:    cp.value,
	}
}

// get gets the cached parameters for the given parameter records. It returns
// the cached parameter only if the parameter record indicates that the cached
// parameter is up-to-date.
func (pc *parameterCache) get(records ...ParameterRecord) (found []cachedParameter, notFound []string) {
	pc.mu.RLock()
	defer pc.mu.RUnlock()

	var cachedParams []cachedParameter
	for _, r := range records {
		p, ok := pc.cache[r.Name]
		isStaleEntry := !utility.IsZeroTime(r.LastUpdated) && r.LastUpdated.After(p.lastUpdated)
		if !ok || isStaleEntry {
			notFound = append(notFound, r.Name)
			continue
		}
		cachedParams = append(cachedParams, p)
	}
	return cachedParams, notFound
}

// put adds the given cached parameters to the cache. If the cached parameter
// already exists and the one being newly cached is more up-to-date than the one
// in the cache, it is overwritten.
func (pc *parameterCache) put(cachedParams ...cachedParameter) {
	pc.mu.Lock()
	defer pc.mu.Unlock()

	for _, cachedParam := range cachedParams {
		existingCachedParam, ok := pc.cache[cachedParam.name]
		if ok && existingCachedParam.lastUpdated.After(cachedParam.lastUpdated) {
			// A more up-to-date parameter is already in the cache.
			continue
		}

		pc.cache[cachedParam.name] = cachedParam
	}
}
