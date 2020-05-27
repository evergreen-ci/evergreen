package poplar

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// BenchmarkSuite is a convenience wrapper around a group of suites.
//
// You can use the Standard() function to convert these benchmarks to
// a standard go library benchmark function.
type BenchmarkSuite []*BenchmarkCase

// Validate aggregates the validation of all constituent tests. The
// validation method for case may also modify a case to set better
// defaults. Validate has continue-on-error semantics.
func (s BenchmarkSuite) Validate() error {
	catcher := grip.NewBasicCatcher()

	for _, c := range s {
		catcher.Add(c.Validate())
	}

	return catcher.Resolve()
}

// Add creates and adds a new case to an existing suite and allows
// access to a fluent-style API for declaring and modifying cases.
func (s *BenchmarkSuite) Add() *BenchmarkCase {
	out := &BenchmarkCase{}
	*s = append(*s, out)
	return out
}

// Run executes a suite of benchmarks writing intrarun data
// to an FTDC compressed events stream prefixed with the benchmark
// name. RunBenchmarks has continue-on-error semantics for test
// failures but not for problems configuring data collection
// infrastructure, which are likely to be file-system
// erorrs. RunBenchmark continues running tests after a failure, and
// will aggregate all error messages.
//
// The results data structure will always be populated even in the
// event of an error.
func (s BenchmarkSuite) Run(ctx context.Context, prefix string) (BenchmarkResultGroup, error) {
	registry := NewRegistry()
	catcher := grip.NewBasicCatcher()
	res := make(BenchmarkResultGroup, 0, len(s))

	if err := s.Validate(); err != nil {
		return res, errors.Wrap(err, "invalid benchmark suite")
	}

	for _, test := range s {
		name := test.Name()
		path := filepath.Join(prefix, name+".ftdc")
		recorder, err := registry.Create(name, CreateOptions{
			Path:      path,
			ChunkSize: 1024,
			Streaming: true,
			Dynamic:   true,
			Recorder:  test.Recorder,
		})

		if err != nil {
			catcher.Add(err)
			break
		}

		startAt := time.Now()
		out := test.Run(ctx, recorder)
		out.CompletedAt = time.Now()
		out.StartAt = startAt
		out.ArtifactPath = path

		catcher.Add(out.Error)
		res = append(res, out)
		catcher.Add(registry.Close(name))
	}

	return res, catcher.Resolve()
}

// Standard returns a go standard library benchmark function that you
// can use to run an entire suite. The same recorder instance is
// passed to each test case, which is run as a subtest.
func (s BenchmarkSuite) Standard(registry *RecorderRegistry) func(*testing.B) {
	return func(b *testing.B) {
		for _, test := range s {
			b.Run(test.Name(), test.Standard(registry))
		}
	}
}
