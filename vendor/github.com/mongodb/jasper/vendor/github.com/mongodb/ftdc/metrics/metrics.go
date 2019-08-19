// Package metrics includes data types used for Golang runtime and
// system metrics collection
package metrics

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
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
	OutputFilePrefix   string
	SampleCount        int
	FlushInterval      time.Duration
	CollectionInterval time.Duration
	SkipGolang         bool
	SkipSystem         bool
	SkipProcess        bool
}

func (opts *CollectOptions) generate(id int) *Runtime {
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

	return out

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
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(opts.FlushInterval < time.Millisecond,
		"flush interval must be greater than a millisecond")
	catcher.NewWhen(opts.CollectionInterval < time.Millisecond,
		"collection interval must be greater than a millisecond")
	catcher.NewWhen(opts.CollectionInterval > opts.FlushInterval,
		"collection interval must be smaller than flush interval")
	catcher.NewWhen(opts.SampleCount < 10, "sample count must be at least 10")
	catcher.NewWhen(opts.SkipGolang && opts.SkipProcess && opts.SkipSystem,
		"cannot skip all metrics collection, must specify golang, process, or system")

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
			grip.Info("collection aborted, flushing results")
			return errors.WithStack(flusher())
		case <-collectTimer.C:
			if err := collector.Add(opts.generate(collectCount)); err != nil {
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
