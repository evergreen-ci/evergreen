package message

import (
	"runtime"
	"sync"
	"time"

	"github.com/mongodb/grip/level"
)

var goStatsCache *goStats

func init() { goStatsCache = &goStats{} }

type goStatsData struct {
	previous int64
	current  int64
}

func (d goStatsData) diff() int64 { return d.current - d.previous }

type goStats struct {
	cgoCalls           goStatsData
	mallocCounter      goStatsData
	freesCounter       goStatsData
	gcRate             goStatsData
	gcPause            uint64
	lastGC             time.Time
	lastCollection     time.Time
	durSinceLastUpdate time.Duration

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

	s.durSinceLastUpdate = now.Sub(s.lastCollection)
	s.lastCollection = now

	return &m
}

type statRate struct {
	Delta    int64         `bson:"delta" json:"delta" yaml:"delta"`
	Duration time.Duration `bson:"duration" json:"duration" yaml:"duration"`
}

func (s *goStats) getRate(stat int64) statRate {
	if s.durSinceLastUpdate == 0 {
		return statRate{}
	}

	return statRate{Delta: stat, Duration: s.durSinceLastUpdate}
}

func (s *goStats) cgo() statRate     { return s.getRate(s.cgoCalls.diff()) }
func (s *goStats) mallocs() statRate { return s.getRate(s.mallocCounter.diff()) }
func (s *goStats) frees() statRate   { return s.getRate(s.freesCounter.diff()) }
func (s *goStats) gcs() statRate     { return s.getRate(s.gcRate.diff()) }

func (s statRate) float() float64 {
	if s.Duration == 0 {
		return 0
	}
	return float64(s.Delta) / float64(s.Duration)
}
func (s statRate) int() int64 {
	if s.Duration == 0 {
		return 0
	}
	return s.Delta / int64(s.Duration)
}

// CollectBasicGoStats returns some very basic runtime statistics about the
// current go process, using runtime.MemStats and
// runtime.NumGoroutine.
//
// The data reported for the runtime event metrics (e.g. mallocs,
// frees, gcs, and cgo calls,) are the counts since the last time
// metrics were collected, and are reported as rates calculated since
// the last time the statics were collected.
//
// Values are cached between calls, to produce the deltas. For the
// best results, collect these messages on a regular interval.
//
// Internally, this uses message.Fields message type, which means the
// order of the fields when serialized is not defined and applications
// cannot manipulate the Raw value of this composer.
//
// The basic idea is taken from https://github.com/YoSmudge/go-stats.
func CollectBasicGoStats() Composer {
	goStatsCache.Lock()
	defer goStatsCache.Unlock()
	m := goStatsCache.update()

	return MakeFields(Fields{
		"memory.objects.HeapObjects":  m.HeapObjects,
		"memory.summary.Alloc":        m.Alloc,
		"memory.summary.System":       m.HeapSys,
		"memory.heap.Idle":            m.HeapIdle,
		"memory.heap.InUse":           m.HeapInuse,
		"memory.counters.mallocs":     goStatsCache.mallocs().float(),
		"memory.counters.frees":       goStatsCache.frees().float(),
		"gc.rate.perSecond":           goStatsCache.gcs().float(),
		"gc.pause.sinceLast.span":     int64(time.Since(goStatsCache.lastGC)),
		"gc.pause.sinceLast.string":   time.Since(goStatsCache.lastGC).String(),
		"gc.pause.last.duration.span": goStatsCache.gcPause,
		"gc.pause.last.span":          time.Duration(goStatsCache.gcPause).String(),
		"goroutines.total":            runtime.NumGoroutine(),
		"cgo.calls":                   goStatsCache.cgo().float(),
	})
}

// GoRuntimeInfo provides
type GoRuntimeInfo struct {
	HeapObjects uint64        `bson:"memory.objects.heap" json:"memory.objects.heap" yaml:"memory.objects.heap"`
	Alloc       uint64        `bson:"memory.summary.alloc" json:"memory.summary.alloc" yaml:"memory.summary.alloc"`
	HeapSystem  uint64        `bson:"memory.summary.system" json:"memory.summary.system" yaml:"memory.summary.system"`
	HeapIdle    uint64        `bson:"memory.heap.idle" json:"memory.heap.idle" yaml:"memory.heap.idle"`
	HeapInUse   uint64        `bson:"memory.heap.used" json:"memory.heap.used" yaml:"memory.heap.used"`
	Mallocs     int64         `bson:"memory.counters.mallocs" json:"memory.counters.mallocs" yaml:"memory.counters.mallocs"`
	Frees       int64         `bson:"memory.counters.frees" json:"memory.counters.frees" yaml:"memory.counters.frees"`
	GC          int64         `bson:"gc.rate" json:"gc.rate" yaml:"gc.rate"`
	GCPause     time.Duration `bson:"gc.pause.duration.last" json:"gc.pause.last" yaml:"gc.pause.last"`
	GCLatency   time.Duration `bson:"gc.pause.duration.latency" json:"gc.pause.duration.latency" yaml:"gc.pause.duration.latency"`
	Goroutines  int64         `bson:"goroutines.total" json:"goroutines.total" yaml:"goroutines.total"`
	CgoCalls    int64         `bson:"cgo.calls" json:"cgo.calls" yaml:"cgo.calls"`

	Message string `bson:"message" json:"message" yaml:"message"`
	Base    `json:"metadata,omitempty" bson:"metadata,omitempty" yaml:"metadata,omitempty"`

	loggable  bool
	useDeltas bool
	useRates  bool
	rendered  string
}

// CollectGoStatsTotals constructs a Composer, which is a
// GoRuntimeInfo internally, that contains data collected from the Go
// runtime about the state of memory use and garbage collection.
//
// The data reported for the runtime event metrics (e.g. mallocs,
// frees, gcs, and cgo calls,) are totals collected since the
// beginning on the runtime.
func CollectGoStatsTotals() Composer {
	s := &GoRuntimeInfo{}
	s.build()

	return s
}

// MakeGoStatsTotals has the same semantics as CollectGoStatsTotals,
// but additionally allows you to set a message string to annotate the
// data.
func MakeGoStatsTotals(msg string) Composer {
	s := &GoRuntimeInfo{Message: msg}
	s.build()

	return s
}

// NewGoStatsTotals has the same semantics as CollectGoStatsTotals,
// but additionally allows you to set a message string and log level
// to annotate the data.
func NewGoStatsTotals(p level.Priority, msg string) Composer {
	s := &GoRuntimeInfo{Message: msg}
	s.build()
	_ = s.SetPriority(p)
	return s
}

// CollectGoStatsDeltas constructs a Composer, which is a
// GoRuntimeInfo internally, that contains data collected from the Go
// runtime about the state of memory use and garbage collection.
//
// The data reported for the runtime event metrics (e.g. mallocs,
// frees, gcs, and cgo calls,) are the counts since the last time
// metrics were collected.
//
// Values are cached between calls, to produce the deltas. For the
// best results, collect these messages on a regular interval.
func CollectGoStatsDeltas() Composer {
	s := &GoRuntimeInfo{useDeltas: true}
	s.build()

	return s
}

// MakeGoStatsDeltas has the same semantics as CollectGoStatsDeltas,
// but additionally allows you to set a message string to annotate the
// data.
func MakeGoStatsDeltas(msg string) Composer {
	s := &GoRuntimeInfo{Message: msg, useDeltas: true}
	s.build()
	return s
}

// NewGoStatsDeltas has the same semantics as CollectGoStatsDeltas,
// but additionally allows you to set a message string to annotate the
// data.
func NewGoStatsDeltas(p level.Priority, msg string) Composer {
	s := &GoRuntimeInfo{Message: msg, useDeltas: true}
	s.build()
	_ = s.SetPriority(p)
	return s
}

// CollectGoStatsRates constructs a Composer, which is a
// GoRuntimeInfo internally, that contains data collected from the Go
// runtime about the state of memory use and garbage collection.
//
// The data reported for the runtime event metrics (e.g. mallocs,
// frees, gcs, and cgo calls,) are the counts since the last time
// metrics were collected, divided by the time since the last
// time the metric was collected, to produce a rate, which is
// calculated using integer division.
//
// For the best results, collect these messages on a regular interval.
func CollectGoStatsRates() Composer {
	s := &GoRuntimeInfo{useRates: true}
	s.build()

	return s
}

// MakeGoStatsRates has the same semantics as CollectGoStatsRates,
// but additionally allows you to set a message string to annotate the
// data.
func MakeGoStatsRates(msg string) Composer {
	s := &GoRuntimeInfo{Message: msg, useRates: true}
	s.build()
	return s
}

// NewGoStatsRates has the same semantics as CollectGoStatsRates,
// but additionally allows you to set a message string to annotate the
// data.
func NewGoStatsRates(p level.Priority, msg string) Composer {
	s := &GoRuntimeInfo{Message: msg, useRates: true}
	s.build()
	_ = s.SetPriority(p)
	return s
}

func (s *GoRuntimeInfo) doCollect() { _ = s.Collect() }

func (s *GoRuntimeInfo) build() {
	goStatsCache.Lock()
	defer goStatsCache.Unlock()
	m := goStatsCache.update()

	s.HeapObjects = m.HeapObjects
	s.Alloc = m.Alloc
	s.HeapSystem = m.HeapSys
	s.HeapIdle = m.HeapIdle
	s.HeapInUse = m.HeapInuse
	s.Goroutines = int64(runtime.NumGoroutine())

	s.GCLatency = time.Since(goStatsCache.lastGC)
	s.GCPause = time.Duration(goStatsCache.gcPause)

	if s.useDeltas {
		s.Mallocs = goStatsCache.mallocs().Delta
		s.Frees = goStatsCache.frees().Delta
		s.GC = goStatsCache.gcs().Delta
		s.CgoCalls = goStatsCache.cgo().Delta
	} else if s.useRates {
		s.Mallocs = goStatsCache.mallocs().int()
		s.Frees = goStatsCache.frees().int()
		s.GC = goStatsCache.gcs().int()
		s.CgoCalls = goStatsCache.cgo().int()
	} else {
		s.Mallocs = goStatsCache.mallocCounter.current
		s.Frees = goStatsCache.freesCounter.current
		s.GC = goStatsCache.gcRate.current
		s.CgoCalls = goStatsCache.cgoCalls.current
	}

	s.loggable = true
}

// Loggable returns true when the GoRuntimeInfo structure is
// populated. Loggable is part of the Composer interface.
func (s *GoRuntimeInfo) Loggable() bool { return s.loggable }

// Raw is part of the Composer interface and returns the GoRuntimeInfo
// object itself.
func (s *GoRuntimeInfo) Raw() interface{} { s.doCollect(); return s }
func (s *GoRuntimeInfo) String() string {
	s.doCollect()

	if s.rendered == "" {
		s.rendered = renderStatsString(s.Message, s)
	}

	return s.rendered
}
