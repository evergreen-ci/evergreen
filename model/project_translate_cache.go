package model

import (
	"fmt"
	"sync/atomic"

	"golang.org/x/sync/singleflight"
)

var translationGroupPtr atomic.Pointer[singleflight.Group]

func init() {
	translationGroupPtr.Store(&singleflight.Group{})
}

// getOrComputeTranslation coalesces concurrent calls for the same key into a
// single compute via singleflight. Errors are not retained after the call
// completes; the next caller will retry compute.
func getOrComputeTranslation(key string, compute func() (*Project, error)) (*Project, error) {
	v, err, _ := translationGroupPtr.Load().Do(key, func() (any, error) {
		return compute()
	})
	if err != nil {
		return nil, err
	}
	return v.(*Project), nil
}

// versionTranslationKey returns the singleflight/cache key for a version-based translation.
func versionTranslationKey(versionID string, preGeneration bool) string {
	return fmt.Sprintf("v:%s:%v", versionID, preGeneration)
}
