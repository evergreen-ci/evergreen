package message

import (
	"runtime"
	"sync"
	"time"
)

var goStatsCache *goStats

func init() { goStatsCache = &goStats{} }

type goStatsData struct {
	previous int64
	current  int64
}

func (d goStatsData) diff() float64 { return float64(d.current - d.previous) }

type goStats struct {
	cgoCalls               goStatsData
	mallocCounter          goStatsData
	freesCounter           goStatsData
	gcRate                 goStatsData
	gcPause                uint64
	lastGC                 time.Time
	lastCollection         time.Time
	secondsSinceLastUpdate float64

	sync.Mutex
}

func (s *goStats) update() *runtime.MemStats {
	now := time.Now()

	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	s.lastGC = time.Unix(0, int64(m.LastGC))
	s.gcPause = m.PauseNs[(m.NumGC+255)%256]

	s.cgoCalls.previous = s.cgoCalls.current
	s.cgoCalls.current = runtime.NumCgoCall()

	s.mallocCounter.previous = s.mallocCounter.current
	s.mallocCounter.current = int64(m.Mallocs)

	s.freesCounter.previous = s.freesCounter.current
	s.freesCounter.current = int64(m.Frees)

	s.gcRate.previous = s.gcRate.current
	s.gcRate.current = int64(m.NumGC)

	s.secondsSinceLastUpdate = now.Sub(s.lastCollection).Seconds()
	s.lastCollection = now

	return &m
}

func (s *goStats) changePerSecond(stat float64) float64 {
	if s.secondsSinceLastUpdate == 0 {
		return float64(0)
	}

	return stat / s.secondsSinceLastUpdate
}

func (s *goStats) cgo() float64     { return s.changePerSecond(s.cgoCalls.diff()) }
func (s *goStats) mallocs() float64 { return s.changePerSecond(s.mallocCounter.diff()) }
func (s *goStats) frees() float64   { return s.changePerSecond(s.freesCounter.diff()) }
func (s *goStats) gcs() float64     { return s.changePerSecond(s.gcRate.diff()) }

// CollectGoStats returns some very basic runtime statistics about the
// current go process, using runtimeMemStats and runtime.NumGoroutine.
//
// Internally, this uses message.Fields{}. Call this at most once a second. Values
// are cached between calls: if you use this function, do not call it
// more than once a second.
//
// The basic idea is taken from https://github.com/YoSmudge/go-stats,.
func CollectGoStats() Composer {
	goStatsCache.Lock()
	defer goStatsCache.Unlock()
	m := goStatsCache.update()

	return MakeFields(Fields{
		"memory.objects.HeapObjects":  m.HeapObjects,
		"memory.summary.Alloc":        m.Alloc,
		"memory.summary.System":       m.HeapSys,
		"memory.heap.Idle":            m.HeapIdle,
		"memory.heap.InUse":           m.HeapInuse,
		"memory.counters.mallocs":     goStatsCache.mallocs(),
		"memory.counters.frees":       goStatsCache.frees(),
		"gc.rate.perSecond":           goStatsCache.gcs(),
		"gc.pause.sinceLast.span":     int64(time.Since(goStatsCache.lastGC)),
		"gc.pause.sinceLast.string":   time.Since(goStatsCache.lastGC).String(),
		"gc.pause.last.duration.span": goStatsCache.gcPause,
		"gc.pause.last.span":          time.Duration(goStatsCache.gcPause).String(),
		"goroutines.total":            runtime.NumGoroutine(),
		"cgo.calls":                   goStatsCache.cgo(),
	})
}
