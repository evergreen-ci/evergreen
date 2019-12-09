package benchmarks

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/evergreen-ci/poplar"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
)

// RunLogging runs the logging benchmark suite.
func RunLogging(ctx context.Context) error {
	prefix := filepath.Join(
		"build",
		fmt.Sprintf("jasper-log-benchmark-%d", time.Now().Unix()),
	)
	if err := os.Mkdir(prefix, os.ModePerm); err != nil {
		return errors.Wrap(err, "problem creating benchmark directory")
	}

	resultFile, err := os.Create(filepath.Join(prefix, "results.txt"))
	if err != nil {
		return errors.Wrap(err, "problem creating result file")
	}

	var resultText string
	s := getLogBenchmarkSuite()
	res, err := s.Run(ctx, prefix)
	if err != nil {
		resultText = fmt.Sprintf("--- FAIL: %s\n", err)
	} else {
		resultText = fmt.Sprintf("--- PASS: %s\n", res.Report())
	}

	catcher := grip.NewBasicCatcher()
	_, err = resultFile.WriteString(resultText)
	catcher.Add(errors.Wrap(err, "failed to write benchmark results to file"))
	catcher.Add(resultFile.Close())

	return catcher.Resolve()
}

type makeProcess func(context.Context, *options.Create) (jasper.Process, error)

func procMap() map[string]func(context.Context, *options.Create) (jasper.Process, error) {
	return map[string]func(context.Context, *options.Create) (jasper.Process, error){
		"Basic": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			opts.Implementation = options.ProcessImplementationBasic
			return jasper.NewProcess(ctx, opts)
		},
		"Blocking": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			opts.Implementation = options.ProcessImplementationBlocking
			return jasper.NewProcess(ctx, opts)
		},
		"BasicSynchronized": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			opts.Implementation = options.ProcessImplementationBasic
			opts.Synchronized = true
			return jasper.NewProcess(ctx, opts)
		},
		"BlockingSynchronized": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			opts.Implementation = options.ProcessImplementationBlocking
			opts.Synchronized = true
			return jasper.NewProcess(ctx, opts)
		},
	}
}

func runIteration(ctx context.Context, makeProc makeProcess, opts *options.Create) error {
	proc, err := makeProc(ctx, opts)
	if err != nil {
		return err
	}
	exitCode, err := proc.Wait(ctx)
	if err != nil && !proc.Info(ctx).Timeout {
		return errors.Wrapf(err, "process with id '%s' exited unexpectedly with code %d", proc.ID(), exitCode)
	}
	return nil
}

func makeCreateOpts(timeout time.Duration, logger options.Logger) *options.Create {
	opts := testutil.YesCreateOpts(timeout)
	opts.Output.Loggers = []options.Logger{logger}
	return opts
}

func getInMemoryLoggerBenchmark(makeProc makeProcess, timeout time.Duration) poplar.Benchmark {
	var logType options.LogType = options.LogInMemory
	logOptions := options.Log{InMemoryCap: 1000, Format: options.LogFormatPlain}
	opts := makeCreateOpts(timeout, options.Logger{Type: logType, Options: logOptions})

	return func(ctx context.Context, r poplar.Recorder, _ int) error {
		startAt := time.Now()
		r.Begin()
		err := runIteration(ctx, makeProc, opts)
		if err != nil {
			return err
		}
		r.IncOps(1)
		sender, err := opts.Output.Loggers[0].Configure()
		if err != nil {
			return err
		}
		rawSender := sender.(*send.InMemorySender)
		r.IncSize(rawSender.TotalBytesSent())
		r.End(time.Since(startAt))

		return nil
	}
}

func getFileLoggerBenchmark(makeProc makeProcess, timeout time.Duration) poplar.Benchmark {
	return func(ctx context.Context, r poplar.Recorder, _ int) error {
		var logType options.LogType = options.LogFile
		file, err := ioutil.TempFile("", "bench_out.txt")
		if err != nil {
			return err
		}
		defer os.Remove(file.Name())
		logOptions := options.Log{FileName: file.Name(), Format: options.LogFormatPlain}
		opts := makeCreateOpts(timeout, options.Logger{Type: logType, Options: logOptions})

		startAt := time.Now()
		r.Begin()
		err = runIteration(ctx, makeProc, opts)
		if err != nil {
			return err
		}
		r.IncOps(1)
		info, err := file.Stat()
		if err != nil {
			return err
		}
		r.IncSize(info.Size())
		r.End(time.Since(startAt))

		return nil
	}
}

func logBenchmarks() map[string]func(makeProcess, time.Duration) poplar.Benchmark {
	return map[string]func(makeProcess, time.Duration) poplar.Benchmark{
		"InMemoryLogger": getInMemoryLoggerBenchmark,
		"FileLogger":     getFileLoggerBenchmark,
	}
}

func getLogBenchmarkSuite() poplar.BenchmarkSuite {
	benchmarkSuite := poplar.BenchmarkSuite{}
	for procName, makeProc := range procMap() {
		for logName, logBench := range logBenchmarks() {
			benchmarkSuite = append(benchmarkSuite,
				&poplar.BenchmarkCase{
					CaseName:         fmt.Sprintf("%s-%s-Send1Second", logName, procName),
					Bench:            logBench(makeProc, time.Second),
					MinRuntime:       30 * time.Second,
					MaxRuntime:       time.Minute,
					Timeout:          10 * time.Minute,
					IterationTimeout: time.Minute,
					Count:            1,
					MinIterations:    10,
					MaxIterations:    20,
					Recorder:         poplar.RecorderPerf,
				},
				&poplar.BenchmarkCase{
					CaseName:         fmt.Sprintf("%s-%s-Send5Seconds", logName, procName),
					Bench:            logBench(makeProc, 5*time.Second),
					MinRuntime:       30 * time.Second,
					MaxRuntime:       time.Minute,
					Timeout:          10 * time.Minute,
					IterationTimeout: time.Minute,
					Count:            1,
					MinIterations:    5,
					MaxIterations:    20,
					Recorder:         poplar.RecorderPerf,
				},
				&poplar.BenchmarkCase{
					CaseName:         fmt.Sprintf("%s-%s-Send30Seconds", logName, procName),
					Bench:            logBench(makeProc, 30*time.Second),
					MinRuntime:       30 * time.Second,
					MaxRuntime:       time.Minute,
					Timeout:          10 * time.Minute,
					IterationTimeout: time.Minute,
					Count:            1,
					MinIterations:    1,
					MaxIterations:    20,
					Recorder:         poplar.RecorderPerf,
				},
			)
		}
	}

	return benchmarkSuite
}
