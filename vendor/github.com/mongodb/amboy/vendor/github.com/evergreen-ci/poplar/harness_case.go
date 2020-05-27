package poplar

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mongodb/ftdc/events"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// BenchmarkCase describes a single benchmark, and describes how to
// run a benchmark, including minimum and maximum runtimes and
// iterations.
//
// With poplar's exceution, via the Run method, cases will execute
// until both the minimum runtime and iteration count are reached, and
// will end as soon as either the maximum iteration or runtime counts
// are exceeded.
//
// You can also use the Standard() function to convert the
// BenchmarkCase into a more conventional go standard library
// Bencharmk function.
//
// You can construct BenchmarkCases either directly, or using a fluent
// interface. The Validate method ensures that the case is well formed
// and sets some default values, overriding unambigious and unuseful
// zero values.
type BenchmarkCase struct {
	CaseName         string
	Bench            Benchmark
	MinRuntime       time.Duration
	MaxRuntime       time.Duration
	Timeout          time.Duration
	IterationTimeout time.Duration
	Count            int
	MinIterations    int
	MaxIterations    int
	Recorder         RecorderType
}

// Recorder is an alias for events.Recorder, eliminating any dependency on ftdc
// for external users of poplar.
type Recorder events.Recorder

// Benchmark defines a function signature for running a benchmark
// test.
//
// These functions take a context to support course timeouts, a
// Recorder instance to capture intra-test data, and a count number to
// tell the test the number of times the function should run.
//
// In general, functions should resemble the following:
//
//    func(ctx context.Context, r events.Recorder, count int) error {
//         ticks := 0
//         for i :=0, i < count; i++ {
//             r.Begin()
//             ticks += 4
//             r.IncOps(4)
//             r.End()
//         }
//    }
//
type Benchmark func(context.Context, Recorder, int) error

func (bench Benchmark) standard(recorder events.Recorder, closer func() error) func(*testing.B) {
	return func(b *testing.B) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		defer func() {
			if err := closer(); err != nil {
				b.Fatal(errors.Wrap(err, "benchmark cleanup"))
			}
		}()

		shim := events.NewShimRecorder(recorder, b)
		b.ResetTimer()
		err := bench(ctx, shim, b.N)
		b.StopTimer()
		if err != nil {
			b.Fatal(errors.Wrap(err, "benchmark failed"))
		}
	}
}

// Standard produces a standard library test function, and configures
// a recorder from the registry.
func (c *BenchmarkCase) Standard(registry *RecorderRegistry) func(*testing.B) {
	return func(b *testing.B) {
		if err := c.Validate(); err != nil {
			b.Fatal(errors.Wrap(err, "benchmark validation failed"))
		}
		test := registry.MakeBenchmark(c)
		test(b)
	}
}

// Name returns either the CaseName value OR the name of the symbol
// for the benchmark function. Use the CaseName field/SetName function
// when you define the case as a function literal, or to override the
// function name.
func (c *BenchmarkCase) Name() string {
	if c.CaseName != "" {
		return c.CaseName
	}
	return getName(c.Bench)
}

// SetName sets the case's name, overriding the symbol name if
// needed, and is part of the BenchmarkCase's fluent interface.
func (c *BenchmarkCase) SetName(n string) *BenchmarkCase { c.CaseName = n; return c }

// SetRecorder overrides, the default event recorder type, which
// allows you to change the way that intrarun data is collected and
// allows you to use histogram data if needed for longer runs, and is
// part of the BenchmarkCase's fluent interface.
func (c *BenchmarkCase) SetRecorder(r RecorderType) *BenchmarkCase { c.Recorder = r; return c }

// SetBench allows you set the benchmark cunftion, and is part of the
// BenchmarkCase's fluent interface.
func (c *BenchmarkCase) SetBench(b Benchmark) *BenchmarkCase { c.Bench = b; return c }

// SetCount allows you to set the count number passed to the benchmark
// function which should control the number of internal iterations,
// and is part of the BenchmarkCase's fluent interface.
//
// If running as a standard library test, this value is ignored.
func (c *BenchmarkCase) SetCount(v int) *BenchmarkCase { c.Count = v; return c }

// SetMaxDuration allows you to specify a maximum duration for the
// test, and is part of the BenchmarkCase's fluent interface. If the
// test has been running for more than this period of time, then it
// will stop running. If you do not specify a timeout, poplar
// execution will use some factor of the max duration.
func (c *BenchmarkCase) SetMaxDuration(dur time.Duration) *BenchmarkCase { c.MaxRuntime = dur; return c } //nolint: goimports

// SetMaxIterations allows you to specify a maximum number of times
// that the test will execute, and is part of the BenchmarkCase's
// fluent interface. This setting is optional.
//
// The number of iterations refers to the number of time that the test
// case executes, and is not passed to the benchmark (e.g. the count.)
//
// The maximum number of iterations is ignored if minimum runtime is
// not satisfied.
func (c *BenchmarkCase) SetMaxIterations(v int) *BenchmarkCase { c.MaxIterations = v; return c }

// SetIterationTimeout describes the timeout set on the context passed
// to each individual iteration, and is part of the BenchmarkCase's
// fluent interface. It must be less than the total timeout.
//
// See the validation function for information on the default value.
func (c *BenchmarkCase) SetIterationTimeout(dur time.Duration) *BenchmarkCase {
	c.IterationTimeout = dur
	return c
}

// SetTimeout sets the total timeout for the entire case, and is part
// of the BenchmarkCase's fluent interface. It must be greater than
// the iteration timeout.
//
// See the validation function for information on the default value.
func (c *BenchmarkCase) SetTimeout(dur time.Duration) *BenchmarkCase { c.Timeout = dur; return c }

// SetDuration sets the minimum runtime for the case, as part of the
// fluent interfaces. This method also sets a maxmum runtime, which is
// 10x this value when the duration is under a minute, 2x this value
// when the duration is under ten minutes, and 1 minute greater when
// the duration is greater than ten minutes.
//
// You can override the maximum duration separately, if these defaults
// do not make sense for your case.
//
// The minimum and maximum runtime values are optional.
func (c *BenchmarkCase) SetDuration(dur time.Duration) *BenchmarkCase {
	c.MinRuntime = dur
	if dur <= time.Minute {
		c.MaxRuntime = 10 * dur
	} else if dur <= 10*time.Minute {
		c.MaxRuntime = 2 * dur
	} else {
		c.MaxRuntime = time.Minute + dur
	}
	return c
}

// SetIterations sets the minimum iterations for the case. It also
// sets the maximum number of iterations to 10x the this value, which
// you can override with SetMaxIterations.
func (c *BenchmarkCase) SetIterations(v int) *BenchmarkCase {
	c.MinIterations = v
	c.MaxIterations = 10 * v
	return c
}

// String satisfies fmt.Stringer and prints a string representation of
// the case and its values.
func (c *BenchmarkCase) String() string {
	return fmt.Sprintf("name=%s, count=%d, min_dur=%s, max_dur=%s, min_iters=%d, max_iters=%d, timeout=%s, iter_timeout=%s",
		c.Name(), c.Count, c.MinRuntime, c.MaxRuntime, c.MinIterations, c.MaxIterations, c.Timeout, c.IterationTimeout)
}

// Validate checks the values of the case, setting default values when
// possible and returning errors if the settings would lead to an
// impossible execution.
//
// If not set, validate imposes the following defaults: count is set
// to 1; the Timeout is set to 3 times the maximum runtime (if set)
// or 10 minutes; the IterationTimeout is set to twice the maximum
// runtime (if set) or five minutes.
func (c *BenchmarkCase) Validate() error {
	if c.Recorder == "" {
		c.Recorder = RecorderPerf
	}

	if c.Count == 0 {
		c.Count = 1
	}

	if c.Timeout == 0 {
		c.Timeout = 3 * c.MaxRuntime
	}

	if c.Timeout == 0 {
		c.Timeout = 10 * time.Minute
	}

	if c.IterationTimeout == 0 {
		c.IterationTimeout = 2 * c.MaxRuntime
	}

	if c.IterationTimeout == 0 {
		c.IterationTimeout = 5 * time.Minute
	}

	catcher := grip.NewBasicCatcher()

	if c.Bench == nil {
		catcher.Add(errors.New("must specify a valid benchmark"))
	}

	if c.MinIterations == 0 {
		catcher.Add(errors.New("must define a minmum number of iterations"))
	}

	if c.MinRuntime == 0 {
		catcher.Add(errors.New("must define a minmum runtime for the case"))
	}

	if c.MinRuntime >= c.MaxRuntime {
		catcher.Add(errors.New("min runtime must not be >= max runtime "))
	}

	if c.MinIterations >= c.MaxIterations {
		catcher.Add(errors.New("min iterations must not be >= max iterations"))
	}

	if c.IterationTimeout > c.Timeout {
		catcher.Add(errors.New("iteration timeout cannot be longer than case timeout"))
	}

	return catcher.Resolve()
}

// Run executes the benchmark recording interrun data to the
// recorder, and returns the populated result.
//
// The benchmark will be run as many times as needed until both the
// minimum iteration and runtime requirements are satisfied OR the
// maximum runtime or iteration requirements are satisfied.
//
// If the test case errors, the runtime or iteration count are not
// incremented, and the test will return early. Similarly if the
// context is canceled the test will return early, and while the tests
// themselves are passed the context which reflects the execution
// timeout, the tests can choose to propagate that error.
func (c *BenchmarkCase) Run(ctx context.Context, recorder events.Recorder) BenchmarkResult {
	res := &BenchmarkResult{
		Name:       c.Name(),
		Iterations: 0,
		Count:      c.Count,
		StartAt:    time.Now(),
	}

	var cancel context.CancelFunc
	ctx, cancel = context.WithTimeout(ctx, c.Timeout)
	defer cancel()

benchLoop:
	for {
		switch {
		case ctx.Err() != nil:
			break benchLoop
		case c.satisfiedMinimums(res) || c.exceededMaximums(res):
			break benchLoop
		default:
			startAt := time.Now()
			bctx, bcancel := context.WithTimeout(ctx, c.IterationTimeout)
			res.Error = c.Bench(bctx, recorder, c.Count)
			bcancel()

			if res.Error != nil {
				break benchLoop
			}

			res.Runtime += time.Since(startAt)
			res.Iterations++
		}
	}
	res.CompletedAt = time.Now()
	return *res
}

func (c *BenchmarkCase) satisfiedMinimums(res *BenchmarkResult) bool {
	return c.satisfiedMinimumRuntime(res) && c.satisfiedMinimumIterations(res)
}

func (c *BenchmarkCase) exceededMaximums(res *BenchmarkResult) bool {
	return c.exceededMaximumIterations(res) || c.exceededMaximumRuntime(res)
}

func (c *BenchmarkCase) satisfiedMinimumRuntime(res *BenchmarkResult) bool {
	return c.MinRuntime == 0 || res.Runtime >= c.MinRuntime
}

func (c *BenchmarkCase) satisfiedMinimumIterations(res *BenchmarkResult) bool {
	return c.MinIterations > 0 && res.Iterations >= c.MinIterations
}

func (c *BenchmarkCase) exceededMaximumRuntime(res *BenchmarkResult) bool {
	return (c.MaxRuntime > 0 && res.Runtime >= c.MaxRuntime) && (c.MinIterations == 0 || res.Iterations > c.MinIterations)
}

func (c *BenchmarkCase) exceededMaximumIterations(res *BenchmarkResult) bool {
	return (c.MaxIterations > 0 && res.Iterations >= c.MaxIterations) && (c.MinRuntime == 0 || res.Runtime > c.MinRuntime)
}
