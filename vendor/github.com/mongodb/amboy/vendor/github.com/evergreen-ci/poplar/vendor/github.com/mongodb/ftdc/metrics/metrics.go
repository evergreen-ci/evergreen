// Package metrics includes data types used for Golang runtime and
// system metrics collection
package metrics

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/evergreen-ci/birch"
	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/util"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// Runtime provides an aggregated view for
type Runtime struct {
	ID        int                    `json:"id" bson:"id"`
	Timestamp time.Time              `json:"ts" bson:"ts"`
	PID       int                    `json:"pid" bson:"pid"`
	Golang    *message.GoRuntimeInfo `json:"golang,omitempty" bson:"golang,omitempty"`
	System    *message.SystemInfo    `json:"system,omitempty" bson:"system,omitempty"`
	Process   *message.ProcessInfo   `json:"process,omitempty" bson:"process,omitempty"`
}

// CollectOptions are the settings to provide the behavior of
// the collection process process.
type CollectOptions struct {
	FlushInterval         time.Duration
	CollectionInterval    time.Duration
	SkipGolang            bool
	SkipSystem            bool
	SkipProcess           bool
	RunParallelCollectors bool
	SampleCount           int
	Collectors            Collectors
	OutputFilePrefix      string
}

type Collectors []CustomCollector

func (c Collectors) Len() int           { return len(c) }
func (c Collectors) Swap(i, j int)      { c[i], c[j] = c[j], c[i] }
func (c Collectors) Less(i, j int) bool { return c[i].Name < c[j].Name }

type CustomCollector struct {
	Name      string
	Operation func(context.Context) *birch.Document
}

func (opts *CollectOptions) generate(ctx context.Context, id int) *birch.Document {
	pid := os.Getpid()
	out := &Runtime{
		ID:        id,
		PID:       pid,
		Timestamp: time.Now(),
	}

	base := message.Base{}

	if !opts.SkipGolang {
		out.Golang = message.CollectGoStatsTotals().(*message.GoRuntimeInfo)
		out.Golang.Base = base
	}

	if !opts.SkipSystem {
		out.System = message.CollectSystemInfo().(*message.SystemInfo)
		out.System.Base = base
	}

	if !opts.SkipProcess {
		out.Process = message.CollectProcessInfo(int32(pid)).(*message.ProcessInfo)
		out.Process.Base = base
	}

	docb, err := bson.Marshal(out)
	if err != nil {
		panic(err)
	}

	if len(opts.Collectors) == 0 {
		return birch.DC.Reader(docb)
	}

	doc := birch.DC.Make(len(opts.Collectors) + 1).Append(birch.EC.SubDocument("runtime", birch.DC.Reader(docb)))
	if !opts.RunParallelCollectors {
		for _, ec := range opts.Collectors {
			doc.Append(birch.EC.SubDocument(ec.Name, ec.Operation(ctx)))
		}

		return doc
	}

	collectors := make(chan CustomCollector, len(opts.Collectors))
	elems := make(chan *birch.Element, len(opts.Collectors))
	num := runtime.NumCPU()
	if num > len(opts.Collectors) {
		num = len(opts.Collectors)
	}

	for _, coll := range opts.Collectors {
		collectors <- coll
	}
	close(collectors)

	wg := &sync.WaitGroup{}
	for i := 0; i < num; i++ {
		wg.Add(1)
		go func() {
			defer recovery.LogStackTraceAndContinue("ftdc metrics collector")
			defer wg.Done()

			for collector := range collectors {
				elems <- birch.EC.SubDocument(collector.Name, collector.Operation(ctx))
			}
		}()
	}
	wg.Wait()

	for elem := range elems {
		doc.Append(elem)
	}

	return doc.Sorted()
}

// NewCollectOptions creates a valid, populated collection options
// structure, collecting data every minute, rotating files every 24
// hours.
func NewCollectOptions(prefix string) CollectOptions {
	return CollectOptions{
		OutputFilePrefix:   prefix,
		SampleCount:        300,
		FlushInterval:      24 * time.Hour,
		CollectionInterval: time.Second,
	}
}

// Validate checks the Collect option settings and ensures that all
// values are reasonable.
func (opts CollectOptions) Validate() error {
	catcher := util.NewCatcher()

	sort.Stable(opts.Collectors)

	catcher.NewWhen(opts.FlushInterval < time.Millisecond,
		"flush interval must be greater than a millisecond")
	catcher.NewWhen(opts.CollectionInterval < time.Millisecond,
		"collection interval must be greater than a millisecond")
	catcher.NewWhen(opts.CollectionInterval > opts.FlushInterval,
		"collection interval must be smaller than flush interval")
	catcher.NewWhen(opts.SampleCount < 10, "sample count must be at least 10")
	catcher.NewWhen(opts.SkipGolang && opts.SkipProcess && opts.SkipSystem,
		"cannot skip all metrics collection, must specify golang, process, or system")
	catcher.NewWhen(opts.RunParallelCollectors && len(opts.Collectors) == 0,
		"cannot run parallel collectors with no collectors specified")

	return catcher.Resolve()
}

// CollectRuntime starts a blocking background process that that
// collects metrics about the current process, the go runtime, and the
// underlying system.
func CollectRuntime(ctx context.Context, opts CollectOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	outputCount := 0
	collectCount := 0

	file, err := os.Create(fmt.Sprintf("%s.%d", opts.OutputFilePrefix, outputCount))
	if err != nil {
		return errors.Wrap(err, "problem creating initial file")
	}

	collector := ftdc.NewStreamingCollector(opts.SampleCount, file)
	collectTimer := time.NewTimer(0)
	flushTimer := time.NewTimer(opts.FlushInterval)
	defer collectTimer.Stop()
	defer flushTimer.Stop()

	flusher := func() error {
		info := collector.Info()
		if info.SampleCount == 0 {
			return nil
		}

		if err = ftdc.FlushCollector(collector, file); err != nil {
			return errors.WithStack(err)
		}

		if err = file.Close(); err != nil {
			return errors.WithStack(err)
		}

		outputCount++

		file, err = os.Create(fmt.Sprintf("%s.%d", opts.OutputFilePrefix, outputCount))
		if err != nil {
			return errors.Wrap(err, "problem creating subsequent file")
		}

		collector = ftdc.NewStreamingCollector(opts.SampleCount, file)
		return nil
	}

	for {
		select {
		case <-ctx.Done():
			return errors.WithStack(flusher())
		case <-collectTimer.C:
			if err := collector.Add(opts.generate(ctx, collectCount)); err != nil {
				return errors.Wrap(err, "problem collecting results")
			}
			collectCount++
			collectTimer.Reset(opts.CollectionInterval)
		case <-flushTimer.C:
			if err := flusher(); err != nil {
				return errors.WithStack(err)
			}
			flushTimer.Reset(opts.FlushInterval)
		}
	}
}
