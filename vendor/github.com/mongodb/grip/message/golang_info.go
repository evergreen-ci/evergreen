package message

import (
	"runtime"
	"time"
)

// CollectGoStats returns some very basic runtime statistics about the
// current go process, using runtimeMemStats and runtime.NumGoroutine.
//
// Internally, this uses message.Fields{}
//
// The basic idea is taken from https://github.com/YoSmudge/go-stats,
// but without the stateful collection.
func CollectGoStats() Composer {
	m := runtime.MemStats{}
	runtime.ReadMemStats(&m)

	lastGC := time.Now().Add(-time.Duration(m.LastGC))

	return MakeFields(Fields{
		"memory.objects.HeapObjects": m.HeapObjects,
		"memory.summary.Alloc":       m.Alloc,
		"memory.summary.System":      m.HeapSys,
		"memory.heap.Idle":           m.HeapIdle,
		"memory.heap.InUse":          m.HeapInuse,
		"gc.sinceLastNS":             time.Since(lastGC),
		"gc.sinceLast":               time.Since(lastGC).String(),
		"goroutines.total":           runtime.NumGoroutine(),
	})
}
