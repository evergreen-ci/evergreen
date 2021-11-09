package events

import (
	"math/rand"
	"sync"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/pkg/errors"
)

// Collector wraps the ftdc.Collector interface and adds
// specific awareness of the Performance type from this package. These
// collectors should be responsible for cumulative summing of values,
// when appropriate.
//
// In general, implementations should obstruct calls to underlying
// collectors Add() method to avoid confusion, either by panicing or
// by no-oping.
type Collector interface {
	AddEvent(*Performance) error
	ftdc.Collector
}

type basicCumulativeCollector struct {
	ftdc.Collector
	current *Performance
}

// NewBasicCollector produces a collector implementation that adds
// Performance points to the underlying FTDC collector. Counter values
// in the point are added to the previous point so that the values are
// cumulative.
//
// This event Collector implementation captures the maximal amount of
// fidelity and should be used except when dictated by retention
// strategy.
func NewBasicCollector(fc ftdc.Collector) Collector {
	return &basicCumulativeCollector{
		Collector: fc,
	}
}

func (c *basicCumulativeCollector) Add(interface{}) error { return nil }
func (c *basicCumulativeCollector) AddEvent(in *Performance) error {
	if in == nil {
		return errors.New("cannot add nil performance event")
	}
	if c.current == nil {
		c.current = in
		return c.Collector.Add(c.current)
	}

	c.current.Add(in)
	return c.Collector.Add(c.current)
}

type passthroughCollector struct {
	ftdc.Collector
}

// NewPassthroughCollector constructs a collector that does not sum
// Performance events and just passes them directly to the underlying
// collector.
func NewPassthroughCollector(fc ftdc.Collector) Collector {
	return &passthroughCollector{
		Collector: fc,
	}
}

func (c *passthroughCollector) Add(interface{}) error { return nil }
func (c *passthroughCollector) AddEvent(in *Performance) error {
	if in == nil {
		return errors.New("cannot add nil performance event")
	}

	return c.Collector.Add(in)
}

type samplingCollector struct {
	ftdc.Collector
	current *Performance
	sample  int
	count   int
}

// NewSamplingCollector constructs a collector that has the same
// semantics as the basic, adding all sampled documents together, but
// only persisting every n-th sample to the underlying collector.
func NewSamplingCollector(fc ftdc.Collector, n int) Collector {
	return &samplingCollector{
		sample:    n,
		Collector: fc,
	}
}

func (c *samplingCollector) Add(interface{}) error { return nil }
func (c *samplingCollector) AddEvent(in *Performance) error {
	if in == nil {
		return errors.New("cannot add nil performance event")
	}

	if c.current == nil {
		c.current = in
	} else {
		c.current.Add(in)
	}

	shouldCollect := c.count%c.sample == 0
	c.count++

	if shouldCollect {
		return c.Collector.Add(c.current)
	}

	return nil
}

type randSamplingCollector struct {
	percent int
	current *Performance
	ftdc.Collector
}

// NewRandomSamplingCollector constructs a Collector that uses a
// psudorandom number generator (go's standard library math/rand) to
// select how often to record an event. All events are summed. Specify
// a percentage between 1 and 99 as the percent to reflect how many
// events to capture.
func NewRandomSamplingCollector(fc ftdc.Collector, sumAll bool, percent int) Collector {
	return &randSamplingCollector{
		percent:   percent,
		Collector: fc,
	}
}

func (c *randSamplingCollector) Add(interface{}) error { return nil }
func (c *randSamplingCollector) AddEvent(in *Performance) error {
	if in == nil {
		return errors.New("cannot add nil performance event")
	}

	if c.current == nil {
		c.current = in
	} else {
		c.current.Add(in)
	}

	if c.shouldCollect() {
		return c.Collector.Add(c.current)
	}
	return nil
}

func (c *randSamplingCollector) shouldCollect() bool {
	if c.percent > 100 {
		return true
	}

	if c.percent <= 0 {
		return false
	}

	return rand.Intn(101) < c.percent
}

type intervalSamplingCollector struct {
	ftdc.Collector
	dur           time.Duration
	current       *Performance
	lastCollected time.Time
}

// NewIntervalCollector constructs a Collector collapses events as in
// the other collector implementations, but will record at most a
// single event per interval.
func NewIntervalCollector(fc ftdc.Collector, interval time.Duration) Collector {
	return &intervalSamplingCollector{
		Collector: fc,
		dur:       interval,
	}
}

func (c *intervalSamplingCollector) Add(interface{}) error { return nil }
func (c *intervalSamplingCollector) AddEvent(in *Performance) error {
	if in == nil {
		return errors.New("cannot add nil performance event")
	}

	if c.current == nil {
		c.current = in
		c.lastCollected = time.Now()
		return c.Collector.Add(c.current)
	}
	c.current.Add(in)
	if time.Since(c.lastCollected) >= c.dur {
		c.lastCollected = time.Now()
		return c.Collector.Add(c.current)
	}
	return nil
}

type synchronizedCollector struct {
	Collector
	mu sync.RWMutex
}

// NewSynchronizedCollector wraps another collector and wraps all
// required calls with the correct lock.
func NewSynchronizedCollector(coll Collector) Collector {
	return &synchronizedCollector{
		Collector: coll,
	}
}

func (c *synchronizedCollector) Add(in interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Collector.Add(in)
}

func (c *synchronizedCollector) AddEvent(in *Performance) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.Collector.AddEvent(in)
}

func (c *synchronizedCollector) SetMetadata(in interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.Collector.SetMetadata(in)
}

func (c *synchronizedCollector) Resolve() ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	return c.Collector.Resolve()
}

func (c *synchronizedCollector) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Collector.Reset()
}

func (c *synchronizedCollector) Info() ftdc.CollectorInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.Collector.Info()
}
