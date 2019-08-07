package pool

import (
	"context"
	"testing"
	"time"

	"github.com/VividCortex/ewma"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
)

func init() {
	sender := grip.GetSender()
	lvl := send.LevelInfo{
		Threshold: level.Alert,
		Default:   level.Warning,
	}
	grip.Warning(sender.SetLevel(lvl))
}

type poolFactory func() amboy.Runner
type testCaseFunc func(*testing.T)

func makeTestQueue(pool amboy.Runner) amboy.Queue {
	return &QueueTester{
		pool:      pool,
		toProcess: make(chan amboy.Job),
		storage:   make(map[string]amboy.Job),
	}

}

func TestRunnerImplementations(t *testing.T) {
	pools := map[string]func() amboy.Runner{
		"Local":  func() amboy.Runner { return new(localWorkers) },
		"Single": func() amboy.Runner { return new(single) },
		"Noop":   func() amboy.Runner { return new(noopPool) },
		"RateLimitedSimple": func() amboy.Runner {
			return &simpleRateLimited{
				size:     1,
				interval: time.Second,
			}
		},
		"RateLimitedAverage": func() amboy.Runner {
			return &ewmaRateLimiting{
				size:   1,
				period: time.Second,
				target: 5,
				ewma:   ewma.NewMovingAverage(),
			}
		},
		"Abortable": func() amboy.Runner {
			return &abortablePool{
				size: 1,
				jobs: make(map[string]context.CancelFunc),
			}
		},
	}
	cases := map[string]func(poolFactory) testCaseFunc{
		"NotStarted": func(factory poolFactory) testCaseFunc {
			return func(t *testing.T) {
				pool := factory()
				assert.False(t, pool.Started())
			}
		},
		"MutableQueue": func(factory poolFactory) testCaseFunc {
			return func(t *testing.T) {
				pool := factory()
				queue := makeTestQueue(pool)

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				// it's an unconfigured runner without a queue, it should always error
				assert.Error(t, pool.Start(ctx))

				// this should start the queue
				assert.NoError(t, pool.SetQueue(queue))

				// it's cool to start the runner
				assert.NoError(t, pool.Start(ctx))

				// once the runner starts you can't add pools
				assert.Error(t, pool.SetQueue(queue))

				// subsequent calls to start should noop
				assert.NoError(t, pool.Start(ctx))
			}
		},
		"CloseImpactsStateAsExpected": func(factory poolFactory) testCaseFunc {
			return func(t *testing.T) {
				pool := factory()
				queue := makeTestQueue(pool)
				assert.False(t, pool.Started())

				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()

				assert.False(t, pool.Started())
				assert.NoError(t, pool.SetQueue(queue))
				assert.NoError(t, pool.Start(ctx))
				assert.True(t, pool.Started())

				assert.NotPanics(t, func() {
					pool.Close(ctx)
				})

				assert.False(t, pool.Started())
			}
		},
	}

	for poolName, factory := range pools {
		t.Run(poolName, func(t *testing.T) {
			for caseName, test := range cases {
				t.Run(poolName+caseName, test(factory))
			}
		})
	}
}
