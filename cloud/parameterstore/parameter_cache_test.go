package parameterstore

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParameterCache(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, pc *parameterCache){
		"PutAndGetSucceeds": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			cp := cachedParameter{
				name:        "name",
				value:       "value",
				lastUpdated: now,
			}
			pc.put(cp)
			rec := ParameterRecord{
				Name:        cp.name,
				LastUpdated: now,
			}
			found, notFound := pc.get(rec)
			assert.ElementsMatch(t, []cachedParameter{cp}, found)
			assert.Empty(t, notFound)
		},
		"PutOverwritesOlderRecord": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			olderCachedParam := cachedParameter{
				name:        "name",
				value:       "older-value",
				lastUpdated: now.Add(-time.Minute),
			}
			pc.put(olderCachedParam)
			newerCachedParam := cachedParameter{
				name:        "name",
				value:       "newer-value",
				lastUpdated: now,
			}
			pc.put(newerCachedParam)

			rec := ParameterRecord{
				Name:        "name",
				LastUpdated: now.Add(-time.Hour),
			}
			found, notFound := pc.get(rec)
			assert.ElementsMatch(t, []cachedParameter{newerCachedParam}, found, "should return latest cached param")
			assert.Empty(t, notFound)
		},
		"PutDoesNotOverwriteNewerRecord": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			newerCachedParam := cachedParameter{
				name:        "name",
				value:       "newer-value",
				lastUpdated: now,
			}
			pc.put(newerCachedParam)
			olderCachedParam := cachedParameter{
				name:        "name",
				value:       "older-value",
				lastUpdated: now.Add(-time.Minute),
			}
			pc.put(olderCachedParam)

			rec := ParameterRecord{
				Name:        "name",
				LastUpdated: now.Add(-time.Hour),
			}
			found, notFound := pc.get(rec)
			assert.ElementsMatch(t, []cachedParameter{newerCachedParam}, found, "should return latest cached param")
			assert.Empty(t, notFound)
		},
		"GetReturnsNotFoundForOutdatedEntry": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			cp := cachedParameter{
				name:        "name",
				value:       "value",
				lastUpdated: now,
			}
			pc.put(cp)
			rec := ParameterRecord{
				Name:        cp.name,
				LastUpdated: now.Add(time.Hour),
			}
			found, notFound := pc.get(rec)
			assert.Empty(t, found)
			assert.ElementsMatch(t, []string{cp.name}, notFound)
		},
		"GetReturnsOnlyFoundSubsetOfEntries": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			cps := []cachedParameter{
				{
					name:        "name-0",
					value:       "value-0",
					lastUpdated: now,
				},
				{
					name:        "name-1",
					value:       "value-1",
					lastUpdated: now,
				},
				{
					name:        "name-2",
					value:       "value-2",
					lastUpdated: now,
				},
			}
			recs := make([]ParameterRecord, 0, len(cps))
			for i := 0; i < len(cps); i++ {
				recs = append(recs, ParameterRecord{
					Name:        cps[i].name,
					LastUpdated: now,
				})
			}

			for i, cp := range cps {
				pc.put(cp)

				found, notFound := pc.get(recs...)
				assert.Len(t, found, i+1)
				assert.Len(t, notFound, len(cps)-(i+1))
			}
		},
		"GetReturnsOnlyUpToDateSubsetOfEntries": func(t *testing.T, pc *parameterCache) {
			now := time.Now()
			cp0 := cachedParameter{
				name:        "name-0",
				value:       "value-0",
				lastUpdated: now,
			}
			cp1 := cachedParameter{
				name:        "name-1",
				value:       "value-1",
				lastUpdated: now,
			}
			pc.put(cp0)
			pc.put(cp1)

			found, notFound := pc.get(ParameterRecord{
				Name:        cp0.name,
				LastUpdated: now.Add(time.Hour),
			}, ParameterRecord{
				Name:        cp1.name,
				LastUpdated: now,
			})
			assert.ElementsMatch(t, []cachedParameter{cp1}, found, "should not return an entry when parameter record indicates that it's outdated")
			assert.ElementsMatch(t, []string{cp0.name}, notFound, "should return an entry when parameter record indicates it's up-to-date")
		},
		"GetParameterNotInCacheReturnsNotFound": func(t *testing.T, pc *parameterCache) {
			found, notFound := pc.get(ParameterRecord{
				Name:        "nonexistent",
				LastUpdated: time.Now(),
			})
			assert.Empty(t, found)
			assert.ElementsMatch(t, []string{"nonexistent"}, notFound)
		},
		"ConcurrentReadAndWritesAreSafeAndLastWriteWins": func(t *testing.T, pc *parameterCache) {
			const name = "name"

			var wg sync.WaitGroup
			startOps := make(chan struct{})
			now := time.Now()

			for i := 0; i < 100; i++ {
				workerNum := i + 1
				wg.Add(1)
				go func() {
					defer wg.Done()
					<-startOps
					pc.put(cachedParameter{
						name:        name,
						value:       fmt.Sprintf("value-%d", workerNum),
						lastUpdated: now.Add(time.Duration(workerNum) * time.Millisecond),
					})
				}()

				wg.Add(1)
				go func() {
					defer wg.Done()
					<-startOps
					found, notFound := pc.get(ParameterRecord{
						Name:        name,
						LastUpdated: now.Add(100 * time.Hour),
					})
					assert.Empty(t, found)
					assert.Len(t, notFound, 1)
				}()
			}

			close(startOps)

			wg.Wait()

			found, notFound := pc.get(ParameterRecord{
				Name:        name,
				LastUpdated: now,
			})
			assert.Empty(t, notFound)
			require.Len(t, found, 1)
			assert.Equal(t, name, found[0].name)
			assert.Equal(t, "value-100", found[0].value, "expected the most recent updated entry's value to be stored as the final value")
			assert.Equal(t, now.Add(100*time.Millisecond), found[0].lastUpdated, "expected the most recent entry timestamp")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			tCase(t, newParameterCache())
		})
	}
}
