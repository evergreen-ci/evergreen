package poplar

import (
	"context"
	"errors"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/ftdc/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCaseType(t *testing.T) {
	t.Run("SatifiesMinimum", func(t *testing.T) {
		t.Run("ZeroValue", func(t *testing.T) {
			c := BenchmarkCase{}
			r := &BenchmarkResult{}
			// no minimum specified
			assert.True(t, c.satisfiedMinimumRuntime(r))
			assert.False(t, c.satisfiedMinimumIterations(r))
			assert.False(t, c.satisfiedMinimums(r))
		})
		t.Run("RuntimeUnfulfilled", func(t *testing.T) {
			c := BenchmarkCase{MinIterations: 2, MinRuntime: time.Hour}
			r := &BenchmarkResult{Iterations: 2, Runtime: time.Minute}
			assert.False(t, c.satisfiedMinimumRuntime(r))
			assert.True(t, c.satisfiedMinimumIterations(r))
			assert.False(t, c.satisfiedMinimums(r))
		})
		t.Run("PassingCaseIterations", func(t *testing.T) {
			c := BenchmarkCase{MinIterations: 2}
			r := &BenchmarkResult{Iterations: 3}
			assert.True(t, c.satisfiedMinimumRuntime(r))
			assert.True(t, c.satisfiedMinimumIterations(r))
			assert.True(t, c.satisfiedMinimums(r))
		})
		t.Run("PassingCase", func(t *testing.T) {
			c := BenchmarkCase{MinIterations: 2, MinRuntime: time.Minute}
			r := &BenchmarkResult{Iterations: 3, Runtime: 5 * time.Minute}
			assert.True(t, c.satisfiedMinimumRuntime(r))
			assert.True(t, c.satisfiedMinimumIterations(r))
			assert.True(t, c.satisfiedMinimums(r))
		})
		t.Run("RuntimeOnlySatisfied", func(t *testing.T) {
			c := BenchmarkCase{MinRuntime: time.Minute, MinIterations: 0}
			r := &BenchmarkResult{Runtime: time.Hour, Iterations: 1}
			assert.True(t, c.satisfiedMinimumRuntime(r))
			assert.False(t, c.satisfiedMinimumIterations(r))
			assert.False(t, c.satisfiedMinimums(r))
		})
	})
	t.Run("ExceedsMinimums", func(t *testing.T) {
		t.Run("ZeroValue", func(t *testing.T) {
			c := BenchmarkCase{}
			r := &BenchmarkResult{}
			// no minimum specified
			assert.False(t, c.exceededMaximumRuntime(r))
			assert.False(t, c.exceededMaximumIterations(r))
			assert.False(t, c.exceededMaximums(r))
		})
		t.Run("UnderRuntime", func(t *testing.T) {
			c := BenchmarkCase{MaxRuntime: time.Minute}
			r := &BenchmarkResult{Runtime: time.Second}
			// no minimum specified
			assert.False(t, c.exceededMaximumRuntime(r))
			assert.False(t, c.exceededMaximumIterations(r))
			assert.False(t, c.exceededMaximums(r))
		})
		t.Run("OverRuntime", func(t *testing.T) {
			c := BenchmarkCase{MaxRuntime: time.Minute}
			r := &BenchmarkResult{Runtime: time.Hour}
			// no minimum specified
			assert.True(t, c.exceededMaximumRuntime(r))
			assert.False(t, c.exceededMaximumIterations(r))
			assert.True(t, c.exceededMaximums(r))
		})
		t.Run("UnderIterations", func(t *testing.T) {
			c := BenchmarkCase{MaxIterations: 60}
			r := &BenchmarkResult{Iterations: 1}
			// no minimum specified
			assert.False(t, c.exceededMaximumRuntime(r))
			assert.False(t, c.exceededMaximumIterations(r))
			assert.False(t, c.exceededMaximums(r))
		})
		t.Run("OverIterations", func(t *testing.T) {
			c := BenchmarkCase{MaxIterations: 60}
			r := &BenchmarkResult{Iterations: 360}
			// no minimum specified
			assert.False(t, c.exceededMaximumRuntime(r))
			assert.True(t, c.exceededMaximumIterations(r))
			assert.True(t, c.exceededMaximums(r))
		})
	})
	t.Run("Validate", func(t *testing.T) {
		t.Run("SetDefault", func(t *testing.T) {
			c := BenchmarkCase{}
			assert.Zero(t, c)
			assert.Error(t, c.Validate())
			assert.NotZero(t, c)
			assert.Equal(t, RecorderPerf, c.Recorder)
			assert.Equal(t, 1, c.Count)
			assert.Equal(t, 10*time.Minute, c.Timeout)
			assert.Equal(t, 5*time.Minute, c.IterationTimeout)
		})
		for _, test := range []struct {
			Name string
			Case func(*testing.T, BenchmarkCase)
		}{
			{
				Name: "ValidateFixture",
				Case: func(t *testing.T, c BenchmarkCase) {
					assert.NoError(t, c.Validate())
				},
			},
			{
				Name: "InvalidIterationTimeout",
				Case: func(t *testing.T, c BenchmarkCase) {
					c.IterationTimeout = time.Hour
					c.Timeout = time.Minute
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "InvalidRuntimes",
				Case: func(t *testing.T, c BenchmarkCase) {
					c.MinRuntime = time.Hour
					c.MaxRuntime = time.Minute
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "Invaliditerations",
				Case: func(t *testing.T, c BenchmarkCase) {
					c.MinIterations = 1000
					c.MaxIterations = 100
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "MinIterationsMustBeSet",
				Case: func(t *testing.T, c BenchmarkCase) {
					c.MinIterations = 0
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "MinRunttimeMustBeSet",
				Case: func(t *testing.T, c BenchmarkCase) {
					c.MinRuntime = 0
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "StringFormat",
				Case: func(t *testing.T, c BenchmarkCase) {
					assert.NotZero(t, c.String())
					assert.Contains(t, c.String(), "name=1,")
					c.CaseName = "foo"
					assert.Contains(t, c.String(), c.CaseName)
				},
			},
		} {
			t.Run(test.Name, func(t *testing.T) {
				c := BenchmarkCase{
					Bench:         func(_ context.Context, _ Recorder, _ int) error { return nil },
					MinIterations: 10,
					MinRuntime:    time.Minute,
					MaxRuntime:    time.Hour,
					MaxIterations: 100,
				}
				require.NoError(t, c.Validate())
				test.Case(t, c)
			})
		}
	})
	t.Run("Fluent", func(t *testing.T) {
		for _, test := range []struct {
			Name string
			Case func(*testing.T, *BenchmarkCase)
		}{
			{
				Name: "ValidateFixture",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.NotZero(t, c)
					assert.Zero(t, *c)
					assert.Error(t, c.Validate())
				},
			},
			{
				Name: "NameRoundTrip",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, "foo", c.SetName("foo").Name())
				},
			},
			{
				Name: "Recorder",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, RecorderType(RecorderPerfSingle), c.SetRecorder(RecorderPerfSingle).Recorder)
				},
			},
			{
				Name: "Benchmark",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.NotZero(t, c.SetBench(func(_ context.Context, _ Recorder, _ int) error { return nil }).Bench)
				},
			},
			{
				Name: "Count",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, 42, c.SetCount(42).Count)
				},
			},
			{
				Name: "MaxDuration",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, time.Minute, c.SetMaxDuration(time.Minute).MaxRuntime)
				},
			},
			{
				Name: "MaxIterations",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, 42, c.SetMaxIterations(42).MaxIterations)
				},
			},
			{
				Name: "IterationTimeout",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, time.Minute, c.SetIterationTimeout(time.Minute).IterationTimeout)
				},
			},
			{
				Name: "Timeout",
				Case: func(t *testing.T, c *BenchmarkCase) {
					assert.Equal(t, time.Minute, c.SetTimeout(time.Minute).Timeout)
				},
			},
			{
				Name: "Iterations",
				Case: func(t *testing.T, c *BenchmarkCase) {
					c = c.SetIterations(10)
					assert.Equal(t, 10, c.MinIterations)
					assert.Equal(t, 100, c.MaxIterations)
				},
			},
			{
				Name: "SetDurationHour",
				Case: func(t *testing.T, c *BenchmarkCase) {
					c = c.SetDuration(time.Hour)
					assert.Equal(t, time.Hour, c.MinRuntime)
					assert.Equal(t, 61*time.Minute, c.MaxRuntime)
				},
			},
			{
				Name: "SetDurationMinute",
				Case: func(t *testing.T, c *BenchmarkCase) {
					c = c.SetDuration(time.Minute)
					assert.Equal(t, time.Minute, c.MinRuntime)
					assert.Equal(t, 10*time.Minute, c.MaxRuntime)
				},
			},
			{
				Name: "SetDurationSecond",
				Case: func(t *testing.T, c *BenchmarkCase) {
					c = c.SetDuration(time.Second)
					assert.Equal(t, time.Second, c.MinRuntime)
					assert.Equal(t, 10*time.Second, c.MaxRuntime)
				},
			},
			{
				Name: "SetDurationTwoMinute",
				Case: func(t *testing.T, c *BenchmarkCase) {
					c = c.SetDuration(2 * time.Minute)
					assert.Equal(t, 2*time.Minute, c.MinRuntime)
					assert.Equal(t, 4*time.Minute, c.MaxRuntime)
				},
			},
		} {
			t.Run(test.Name, func(t *testing.T) {
				test.Case(t, &BenchmarkCase{})
			})
		}
	})
	t.Run("Execution", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		collector := ftdc.NewBaseCollector(10)
		recorder := events.NewSingleRecorder(collector)

		t.Run("NoopBenchmark", func(t *testing.T) {
			c := BenchmarkCase{
				Bench:            func(ctx context.Context, _ Recorder, count int) error { return nil },
				MinIterations:    2,
				MaxIterations:    4,
				MinRuntime:       time.Millisecond,
				MaxRuntime:       time.Second,
				Timeout:          time.Minute,
				IterationTimeout: time.Microsecond,
			}
			require.NoError(t, c.Validate())
			res := c.Run(ctx, recorder)
			assert.NoError(t, res.Error)
			assert.True(t, res.Runtime >= time.Millisecond)
			assert.True(t, res.Runtime <= time.Second)
			assert.True(t, res.Iterations >= c.MinIterations)
			assert.False(t, res.Iterations <= c.MaxIterations)

			assert.True(t, res.CompletedAt.Sub(res.StartAt) <= time.Second)
			assert.True(t, res.CompletedAt.Sub(res.StartAt) >= time.Millisecond)

		})
		t.Run("CanceledContext", func(t *testing.T) {
			c := BenchmarkCase{
				Bench: func(ctx context.Context, _ Recorder, count int) error {
					return nil
				},
				MinIterations:    2,
				MaxIterations:    4,
				MinRuntime:       time.Minute,
				MaxRuntime:       time.Hour,
				Timeout:          2 * time.Hour,
				IterationTimeout: 20 * time.Second,
			}
			require.NoError(t, c.Validate())
			canceledCtx, canceled := context.WithCancel(ctx)
			canceled()
			res := c.Run(canceledCtx, recorder)
			assert.NoError(t, res.Error)
			assert.Zero(t, res.Iterations)
			assert.True(t, res.CompletedAt.Sub(res.StartAt) < time.Second)
		})
		t.Run("Error", func(t *testing.T) {
			err := errors.New("foo")
			c := BenchmarkCase{
				Bench: func(ctx context.Context, _ Recorder, count int) error {
					return err
				},
				MinIterations:    2,
				MaxIterations:    4,
				MinRuntime:       time.Minute,
				MaxRuntime:       time.Hour,
				Timeout:          2 * time.Hour,
				IterationTimeout: 20 * time.Second,
			}
			require.NoError(t, c.Validate())

			res := c.Run(ctx, recorder)
			assert.Error(t, res.Error)
			assert.Equal(t, err, res.Error)
			assert.Zero(t, res.Iterations)
			assert.True(t, res.CompletedAt.Sub(res.StartAt) < time.Millisecond)
		})
	})
}

func TestResultType(t *testing.T) {
	res := BenchmarkResult{
		Name:         "PoplarBench",
		Runtime:      42 * time.Minute,
		Count:        100,
		Iterations:   400,
		ArtifactPath: "build/poplar_bench.ftdc",
		StartAt:      time.Now().Add(-time.Hour),
		CompletedAt:  time.Now(),
	}
	t.Run("Zero", func(t *testing.T) {
		emptyRes := BenchmarkResult{}
		assert.Zero(t, emptyRes)
		assert.NotZero(t, emptyRes.Report())
	})
	t.Run("PassingContent", func(t *testing.T) {
		report := res.Report()
		assert.NotZero(t, res)
		assert.Contains(t, report, res.Name)
		assert.Contains(t, report, "runtime=42m")
		assert.Contains(t, report, "400")
		assert.Contains(t, report, "100")
		assert.NotContains(t, report, "poplar_bench.ftdc")
		assert.Contains(t, report, "REPORT")
		assert.Contains(t, report, "RUN")
		assert.Contains(t, report, "PASS")
	})
	t.Run("FailingContent", func(t *testing.T) {
		res.Error = errors.New("foo")
		defer func() { res.Error = nil }()

		report := res.Report()
		assert.NotZero(t, res)
		assert.Contains(t, report, res.Name)
		assert.Contains(t, report, "runtime=42m")
		assert.Contains(t, report, "400")
		assert.Contains(t, report, "100")
		assert.NotContains(t, report, "poplar_bench.ftdc")
		assert.Contains(t, report, "REPORT")
		assert.Contains(t, report, "ERRORS")
		assert.Contains(t, report, "RUN")
		assert.Contains(t, report, "FAIL")
	})
	t.Run("Composer", func(t *testing.T) {
		res.Error = errors.New("foo")
		defer func() { res.Error = nil }()

		comp := res.Composer()
		assert.NotNil(t, comp)
		assert.Equal(t, res.Report(), comp.String())
		assert.True(t, comp.Loggable())
		assert.False(t, (&resultComposer{}).Loggable())
		assert.NotNil(t, comp.Raw())
		assert.Equal(t, comp, comp.Raw())
	})
	t.Run("Export", func(t *testing.T) {
		out := res.Export()
		assert.NotZero(t, out)
		assert.Equal(t, res.StartAt, out.CreatedAt)
		assert.Equal(t, res.CompletedAt, out.CompletedAt)
		assert.NotNil(t, res.ArtifactPath)
		assert.Len(t, out.Artifacts, 1)
		assert.Equal(t, res.ArtifactPath, out.Artifacts[0].LocalFile)
	})
	t.Run("Results", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			s := BenchmarkResultGroup{}
			assert.Zero(t, s.Report())
			assert.False(t, s.Composer().Loggable())
			assert.Equal(t, len(s), len(s.Export()))
		})
		t.Run("Populated", func(t *testing.T) {
			s := BenchmarkResultGroup{
				res, res, res,
			}
			assert.NotZero(t, s.Report())
			assert.True(t, s.Composer().Loggable())
			assert.Equal(t, len(s), len(s.Export()))
		})
	})
}

func TestSuiteType(t *testing.T) {
	t.Run("Validation", func(t *testing.T) {
		t.Run("Empty", func(t *testing.T) {
			s := BenchmarkSuite{}
			assert.NoError(t, s.Validate())
		})
		t.Run("Zero", func(t *testing.T) {
			c := BenchmarkCase{}
			assert.Error(t, c.Validate())
			assert.Error(t, c.Validate())
			s := BenchmarkSuite{&c}
			assert.Error(t, s.Validate())
			assert.Equal(t, s.Validate().Error(), c.Validate().Error())
		})
	})
	t.Run("Fluent", func(t *testing.T) {
		s := BenchmarkSuite{}
		assert.Len(t, s, 0)
		c := s.Add().SetName("poplar")
		assert.NotNil(t, c)
		assert.Len(t, s, 1)
		assert.Equal(t, s[0].Name(), c.Name())
	})
	t.Run("Execution", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		tmpdir, err := ioutil.TempDir("", "poplar-harness-test")
		require.NoError(t, err)
		defer func() { require.NoError(t, os.RemoveAll(tmpdir)) }()

		t.Run("Empty", func(t *testing.T) {
			s := BenchmarkSuite{}
			res, err := s.Run(ctx, tmpdir)
			assert.NoError(t, err)
			assert.Len(t, res, 0)
		})
		t.Run("NoopTest", func(t *testing.T) {
			c := &BenchmarkCase{
				Bench:            func(ctx context.Context, _ Recorder, count int) error { return nil },
				MinIterations:    2,
				MaxIterations:    4,
				MinRuntime:       time.Millisecond,
				MaxRuntime:       time.Second,
				Timeout:          time.Minute,
				IterationTimeout: time.Microsecond,
				Recorder:         RecorderPerf,
			}
			s := BenchmarkSuite{c}
			assert.Len(t, s, 1)
			res, err := s.Run(ctx, tmpdir)
			assert.NoError(t, err)
			assert.Len(t, res, 1)
		})
		t.Run("SomeErrors", func(t *testing.T) {
			counter := 0

			s := BenchmarkSuite{
				{
					CaseName:         "firstErr",
					Bench:            func(ctx context.Context, _ Recorder, count int) error { counter++; return nil },
					MinIterations:    2,
					MaxIterations:    4,
					MinRuntime:       time.Millisecond,
					MaxRuntime:       time.Second,
					Timeout:          time.Minute,
					IterationTimeout: time.Microsecond,
					Recorder:         RecorderPerf,
				},
				{
					CaseName:         "errorerrror",
					Bench:            func(ctx context.Context, _ Recorder, count int) error { return errors.New("foo") },
					MinIterations:    2,
					MaxIterations:    4,
					MinRuntime:       time.Millisecond,
					MaxRuntime:       time.Second,
					Timeout:          time.Minute,
					IterationTimeout: time.Microsecond,
					Recorder:         RecorderPerf,
				},
				{
					CaseName:         "seconderr",
					Bench:            func(ctx context.Context, _ Recorder, count int) error { counter += 2; return nil },
					MinIterations:    2,
					MaxIterations:    4,
					MinRuntime:       time.Millisecond,
					MaxRuntime:       time.Second,
					Timeout:          time.Minute,
					IterationTimeout: time.Microsecond,
					Recorder:         RecorderPerf,
				},
			}
			assert.Len(t, s, 3)
			res, err := s.Run(ctx, tmpdir)
			assert.Error(t, err)
			assert.Len(t, res, 3)
			assert.Contains(t, err.Error(), "foo")
			assert.True(t, counter > 50)
		})
		t.Run("CollectoError", func(t *testing.T) {
			s := BenchmarkSuite{
				{
					CaseName:         "one",
					Bench:            func(ctx context.Context, _ Recorder, count int) error { return nil },
					MinIterations:    2,
					MaxIterations:    4,
					MinRuntime:       time.Millisecond,
					MaxRuntime:       time.Second,
					Timeout:          time.Minute,
					IterationTimeout: time.Microsecond,
					Recorder:         RecorderPerf,
				},
				{
					CaseName:         "one",
					Bench:            func(ctx context.Context, _ Recorder, count int) error { return nil },
					MinIterations:    2,
					MaxIterations:    4,
					MinRuntime:       time.Millisecond,
					MaxRuntime:       time.Second,
					Timeout:          time.Minute,
					IterationTimeout: time.Microsecond,
					Recorder:         RecorderPerf,
				},
			}

			assert.Len(t, s, 2)
			res, err := s.Run(ctx, tmpdir)
			assert.Error(t, err)
			assert.Len(t, res, 1)
			assert.Contains(t, err.Error(), "because it exists")
			assert.Contains(t, err.Error(), "could not create")
		})

		t.Run("Standard", func(t *testing.T) {

			registry := NewRegistry()
			registry.SetBenchRecorderPrefix(tmpdir)

			counter := 0
			c := &BenchmarkCase{
				CaseName:         "one-" + randomString(),
				Bench:            func(ctx context.Context, _ Recorder, count int) error { counter++; return nil },
				MinIterations:    2,
				MaxIterations:    4,
				MinRuntime:       time.Millisecond,
				MaxRuntime:       time.Second,
				Timeout:          time.Minute,
				IterationTimeout: time.Microsecond,
				Recorder:         RecorderPerf,
			}

			s := BenchmarkSuite{c}
			assert.NoError(t, c.Validate())
			res := testing.Benchmark(s.Standard(registry))
			assert.Equal(t, 1, res.N)
			assert.True(t, res.T < time.Millisecond)
			first := counter

			c.MinRuntime = time.Minute
			c.CaseName = "two-" + randomString()
			s = BenchmarkSuite{c}
			assert.Error(t, c.Validate())
			res = testing.Benchmark(s.Standard(registry))
			assert.Equal(t, 1, res.N)
			assert.True(t, res.T < time.Millisecond)
			assert.Equal(t, first, counter)
		})
	})

}
