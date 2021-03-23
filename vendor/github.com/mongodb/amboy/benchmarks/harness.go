package benchmarks

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/evergreen-ci/poplar"
	"github.com/google/uuid"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RunQueue runs the queue benchmark suite.
func RunQueue(ctx context.Context) error {
	// Set the level to suppress debug messages from the queue.
	grip.SetLevel(send.LevelInfo{Threshold: level.Info, Default: level.Info})

	prefix := filepath.Join("build", fmt.Sprintf("queue-benchmark-%d", time.Now().Unix()))
	if err := os.MkdirAll(prefix, os.ModePerm); err != nil {
		return errors.Wrap(err, "creating benchmark directory")
	}

	resultFile, err := os.Create(filepath.Join(prefix, "results.txt"))
	if err != nil {
		return errors.Wrap(err, "creating result file")
	}

	var resultText string
	s := queueBenchmarkSuite()
	res, err := s.Run(ctx, prefix)
	if err != nil {
		resultText = err.Error()
	} else {
		resultText = res.Report()
	}

	catcher := grip.NewBasicCatcher()
	_, err = resultFile.WriteString(resultText)
	catcher.Add(errors.Wrap(err, "writing benchmark results to file"))
	catcher.Add(resultFile.Close())

	return catcher.Resolve()
}

type closeFunc func() error

type queueConstructor func(context.Context) (amboy.Queue, closeFunc, error)
type jobConstructor func(id string) amboy.Job

func basicThroughputBenchmark(makeQueue queueConstructor, makeJob jobConstructor, timeout time.Duration) poplar.Benchmark {
	return func(ctx context.Context, r poplar.Recorder, count int) error {
		for i := 0; i < count; i++ {
			if err := runBasicThroughputIteration(ctx, r, makeQueue, makeJob, timeout); err != nil {
				return errors.Wrapf(err, "iteration %d", i)
			}
		}
		return nil
	}
}

func runBasicThroughputIteration(ctx context.Context, r poplar.Recorder, makeQueue queueConstructor, makeJob jobConstructor, timeout time.Duration) error {
	q, closeQueue, err := makeQueue(ctx)
	if err != nil {
		return errors.Wrap(err, "making queue")
	}
	defer func() {
		grip.Error(errors.Wrap(closeQueue(), "cleaning up queue"))
	}()

	errChan := make(chan error)
	qctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		for {
			if qctx.Err() != nil {
				return
			}

			j := makeJob(uuid.New().String())
			if err := q.Put(qctx, j); err != nil {
				select {
				case <-qctx.Done():
					return
				case errChan <- errors.Wrap(err, "adding job to queue"):
					return
				}
			}
		}
	}()

	startAt := time.Now()
	r.BeginIteration()
	defer func() {
		r.EndIteration(time.Since(startAt))
	}()

	if err = q.Start(qctx); err != nil {
		return errors.Wrap(err, "starting queue")
	}

	timer := time.NewTimer(timeout)
	select {
	case err := <-errChan:
		return errors.WithStack(err)
	case <-qctx.Done():
		return qctx.Err()
	case <-timer.C:
		stats := q.Stats(ctx)
		r.IncOperations(int64(stats.Completed))
	}
	return nil
}

func queueBenchmarkSuite() poplar.BenchmarkSuite {
	var suite poplar.BenchmarkSuite
	for queueName, makeQueue := range map[string]queueConstructor{
		"MongoDB": makeMongoDBQueue,
	} {
		for jobName, makeJob := range map[string]jobConstructor{
			"Noop":                     newNoopJob,
			"ScopedNoop":               newScopedNoopJob,
			"MixedScopeAndNoScopeNoop": newSometimesScopedJob(50),
		} {
			suite = append(suite,
				&poplar.BenchmarkCase{
					CaseName:      fmt.Sprintf("%s-%s-15Second", queueName, jobName),
					Bench:         basicThroughputBenchmark(makeQueue, makeJob, 15*time.Second),
					Count:         1,
					MinRuntime:    15 * time.Second,
					MaxRuntime:    5 * time.Minute,
					Timeout:       10 * time.Minute,
					MinIterations: 10,
					MaxIterations: 20,
					Recorder:      poplar.RecorderPerf,
				},
				&poplar.BenchmarkCase{
					CaseName:      fmt.Sprintf("%s-%s-1Minute", queueName, jobName),
					Bench:         basicThroughputBenchmark(makeQueue, makeJob, time.Minute),
					Count:         1,
					MinRuntime:    time.Minute,
					MaxRuntime:    15 * time.Minute,
					Timeout:       30 * time.Minute,
					MinIterations: 10,
					MaxIterations: 20,
					Recorder:      poplar.RecorderPerf,
				},
			)
		}
	}
	return suite
}

func makeMongoDBQueue(ctx context.Context) (amboy.Queue, closeFunc, error) {
	mdbOpts := queue.DefaultMongoDBOptions()
	mdbOpts.DB = "amboy_benchmarks"
	queueOpts := queue.MongoDBQueueCreationOptions{
		Size: runtime.NumCPU(),
		Name: uuid.New().String(),
		MDB:  mdbOpts,
	}
	queue, err := queue.NewMongoDBQueue(ctx, queueOpts)
	if err != nil {
		return nil, nil, errors.WithStack(err)
	}
	cleanupDB := func() error {
		cctx, cancel := context.WithCancel(ctx)
		defer cancel()
		client, err := mongo.NewClient(options.Client().ApplyURI(mdbOpts.URI).SetConnectTimeout(time.Second))
		if err != nil {
			return errors.Wrap(err, "creating mongo client")
		}
		if err := client.Connect(cctx); err != nil {
			return errors.Wrap(err, "connecting to DB")
		}
		defer func() {
			grip.Error(errors.Wrap(client.Disconnect(cctx), "disconnecting client"))
		}()
		if err := client.Database(mdbOpts.DB).Drop(cctx); err != nil {
			return errors.Wrap(err, "cleaning up benchmark database")
		}
		return nil
	}
	return queue, cleanupDB, nil
}

type noopJob struct {
	job.Base
}

func newNoopJobInstance() *noopJob {
	j := &noopJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "benchmark",
				Version: 1,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

func newNoopJob(id string) amboy.Job {
	j := newNoopJobInstance()
	j.SetID(id)
	return j
}

func newScopedNoopJob(id string) amboy.Job {
	j := newNoopJobInstance()
	j.SetScopes([]string{"common_scope"})
	j.SetID(id)
	return j
}

func newSometimesScopedJob(percentScoped int) func(id string) amboy.Job {
	if percentScoped < rand.Intn(100) {
		return newScopedNoopJob
	}
	return newNoopJob
}

func init() {
	registry.AddJobType("benchmark", func() amboy.Job {
		return newNoopJobInstance()
	})
}

func (j *noopJob) Run(ctx context.Context) {
	j.MarkComplete()
}
