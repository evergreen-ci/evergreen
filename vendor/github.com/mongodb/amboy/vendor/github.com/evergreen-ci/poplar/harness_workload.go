package poplar

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

// BenchmarkWorkload provides a way to express a more complex
// performance test, that involves multiple instances of a benchmark
// test running at the same time.
//
// You can specify the workload as either a single benchmark case, or
// as a ordered list of benchmark operations, however it is not valid
// to do both in the same workload instance.
//
// If you specify a group of workload operations when executing,
// poplar will run each sub-workload (with however many instances of
// the workload are specified,) sequentially, with no inter-workload
// synchronization.
type BenchmarkWorkload struct {
	WorkloadName    string
	WorkloadTimeout *time.Duration
	Case            *BenchmarkCase
	Group           []*BenchmarkWorkload
	Instances       int
	Recorder        RecorderType
}

// Validate ensures that the workload is well formed. Additionally,
// ensures that the all cases and workload groups are valid, and have
// the same recorder type defined.
func (w *BenchmarkWorkload) Validate() error {
	catcher := grip.NewBasicCatcher()

	if w.WorkloadTimeout != nil && w.Timeout() < time.Millisecond {
		catcher.Add(errors.New("cannot specify timeout less than a millisecond"))
	}

	if w.Case == nil && w.Group == nil {
		catcher.Add(errors.New("cannot define a workload with out work"))
	}

	if w.Case != nil && w.Group != nil {
		catcher.Add(errors.New("cannot define a workload with both a case and a group"))
	}

	if w.Instances <= 1 {
		catcher.Add(errors.New("must define more than a single instance in a workload"))
	}

	if w.WorkloadName == "" && w.Case == nil {
		catcher.Add(errors.New("must specify a name for a workload"))
	}

	if w.Case != nil {
		catcher.Add(w.Case.Validate())
		w.Case.Recorder = w.Recorder
	}

	for _, gwl := range w.Group {
		catcher.Add(gwl.Validate())
		gwl.Recorder = w.Recorder
	}

	return catcher.Resolve()
}

// Name returns the name of the workload as defined or the name of the
// case if no name is defined.
func (w *BenchmarkWorkload) Name() string {
	if w.WorkloadName != "" {
		return w.WorkloadName
	}

	if w.Case != nil {
		return w.Case.Name()
	}

	return ""
}

// SetName makes it possible to set the name of the workload in a
// chainable context.
func (w *BenchmarkWorkload) SetName(n string) *BenchmarkWorkload { w.WorkloadName = n; return w }

// Timeout returns the timeout for the workload, returning -1 when the
// timeout is unset, and the value otherwise.
func (w *BenchmarkWorkload) Timeout() time.Duration {
	if w.WorkloadTimeout == nil {
		return -1
	}

	return *w.WorkloadTimeout
}

// SetTimeout allows you to define a timeout for the workload as a
// whole. Timeouts are not required and sub-cases or workloads will
// are respected. Additionally, the validation method requires that
// the timeout be greater than 1 millisecond.
func (w *BenchmarkWorkload) SetTimeout(d time.Duration) *BenchmarkWorkload {
	w.WorkloadTimeout = &d
	return w
}

// SetInstances makes it possible to set the Instance value of the
// workload in a chained context.
func (w *BenchmarkWorkload) SetInstances(i int) *BenchmarkWorkload { w.Instances = i; return w }

// SetRecorder overrides, the default event recorder type, which
// allows you to change the way that intrarun data is collected and
// allows you to use histogram data if needed for longer runs, and is
// part of the BenchmarkCase's fluent interface.
func (w *BenchmarkWorkload) SetRecorder(r RecorderType) *BenchmarkWorkload { w.Recorder = r; return w }

// SetCase creates a case for the workload to run, returning it for
// the caller to manipulate. This method also unsets the group.
func (w *BenchmarkWorkload) SetCase() *BenchmarkCase {
	w.Group = nil
	c := &BenchmarkCase{}
	w.Case = c
	return c
}

// Add creates a new sub-workload, and adds it to the workload's
// group. Add also unsets the case, if set.
func (w *BenchmarkWorkload) Add() *BenchmarkWorkload {
	w.Case = nil
	out := &BenchmarkWorkload{}
	w.Group = append(w.Group, out)
	return out
}

// Run executes the workload, and has similar semantics to the
// BenchmarkSuite implementation.
func (w *BenchmarkWorkload) Run(ctx context.Context, prefix string) (BenchmarkResultGroup, error) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()

	if w.WorkloadTimeout != nil {
		ctx, cancel = context.WithTimeout(ctx, w.Timeout())
		defer cancel()
	}

	registry := NewRegistry()
	catcher := grip.NewBasicCatcher()
	wg := &sync.WaitGroup{}
	results := make(chan BenchmarkResult)
	res := BenchmarkResultGroup{}

	go func() {
		defer func() {
			catcher.Add(recovery.AnnotateMessageWithPanicError(recover(), nil, message.Fields{
				"name":     w.Name(),
				"op":       "collecting results",
				"executor": "native",
			}))
		}()

		for {

			select {
			case r := <-results:
				res = append(res, r)
			case <-ctx.Done():
				return
			}
		}
	}()

	for i := 0; i < w.Instances; i++ {
		wg.Add(1)
		go func(instanceIdx int) {
			defer wg.Done()
			defer func() {
				catcher.Add(recovery.AnnotateMessageWithPanicError(recover(), nil, message.Fields{
					"idx":      instanceIdx,
					"name":     w.Name(),
					"executor": "native",
					"op":       "running workload",
				}))
			}()

			name := fmt.Sprintf("%s.%d", w.Name(), instanceIdx)
			path := filepath.Join(prefix, name+".ftdc")
			recorder, err := registry.Create(name, CreateOptions{
				Path:      path,
				ChunkSize: 1024,
				Streaming: true,
				Dynamic:   true,
				Recorder:  w.Recorder,
			})
			if err != nil {
				catcher.Add(err)
				return
			}

			if w.Case != nil {
				select {
				case results <- w.Case.Run(ctx, recorder):
					return
				case <-ctx.Done():
					return
				}
			}

			for idx, wlg := range w.Group {
				resultGroup, err := wlg.Run(ctx, fmt.Sprintf("%s.%s.%d.%d", prefix, name, instanceIdx, idx))
				catcher.Add(err)

				for _, r := range resultGroup {
					select {
					case results <- r:
						continue
					case <-ctx.Done():
						return
					}
				}
			}
		}(i)
	}
	wg.Wait()
	close(results)

	for r := range results {
		r.Workload = true
		if r.Instances == 0 {
			r.Instances = w.Instances
		}
	}

	return res, catcher.Resolve()
}

// Standard produces a standard golang benchmarking function from a
// poplar workload.
//
// These invocations are not able to respect the top-level workload
// timeout, and *do* perform pre-flight workload validation.
func (w *BenchmarkWorkload) Standard(registry *RecorderRegistry) func(*testing.B) {
	return func(b *testing.B) {
		if err := w.Validate(); err != nil {
			b.Fatal(errors.Wrap(err, "benchmark workload failed"))
		}

		wg := &sync.WaitGroup{}
		for i := 0; i < w.Instances; i++ {
			wg.Add(1)
			go func(id int) {
				defer func() {
					err := recovery.AnnotateMessageWithPanicError(recover(), nil, message.Fields{
						"idx":      id,
						"name":     w.Name(),
						"op":       "running workload",
						"executor": "standard",
					})

					if err != nil {
						b.Fatal(err)
					}
				}()

				if w.Case != nil {
					b.Run(fmt.Sprintf("WorkloadCase%s%s#%d", w.WorkloadName, w.Case.Name(), id), w.Case.Standard(registry))
					return
				}

				for idx, wlg := range w.Group {
					b.Run(fmt.Sprintf("WorkloadGroup%s%s%d#%d", w.WorkloadName, wlg.Name(), idx, id), wlg.Standard(registry))
				}
			}(i)
		}
		wg.Done()
	}
}
