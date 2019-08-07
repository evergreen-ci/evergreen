package queue

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/pool"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	mgo "gopkg.in/mgo.v2"
)

const defaultLocalQueueCapcity = 10000

func init() {
	grip.SetName("amboy.queue.tests")
	grip.Error(grip.SetSender(send.MakeNative()))

	lvl := grip.GetSender().Level()
	lvl.Threshold = level.Error
	_ = grip.GetSender().SetLevel(lvl)

	job.RegisterDefaultJobs()
}

func newDriverID() string { return strings.Replace(uuid.NewV4().String(), "-", ".", -1) }

type TestCloser func(context.Context) error

type QueueTestCase struct {
	Name                    string
	Constructor             func(context.Context, int) (amboy.Queue, error)
	MaxSize                 int
	OrderedSupported        bool
	OrderedStartsBefore     bool
	WaitUntilSupported      bool
	DispatchBeforeSupported bool
	SkipUnordered           bool
	IsRemote                bool
	Skip                    bool
}

type DriverTestCase struct {
	Name                    string
	SetDriver               func(context.Context, amboy.Queue, string) (TestCloser, error)
	Constructor             func(context.Context, string, int) ([]Driver, TestCloser, error)
	SupportsLocal           bool
	SupportsMulti           bool
	WaitUntilSupported      bool
	DispatchBeforeSupported bool
	SkipOrdered             bool
	Skip                    bool
}

type PoolTestCase struct {
	Name       string
	SetPool    func(amboy.Queue, int) error
	SkipRemote bool
	SkipMulti  bool
	MinSize    int
	MaxSize    int
}

type MultipleExecutionTestCase struct {
	Name            string
	Setup           func(context.Context, amboy.Queue, amboy.Queue) (TestCloser, error)
	MultipleDrivers bool
}

type SizeTestCase struct {
	Name string
	Size int
}

func DefaultQueueTestCases() []QueueTestCase {
	return []QueueTestCase{
		{
			Name:                    "Local",
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			Constructor:             func(ctx context.Context, size int) (amboy.Queue, error) { return NewLocalUnordered(size), nil },
		},
		{
			Name:                    "AdaptiveOrdering",
			OrderedSupported:        true,
			OrderedStartsBefore:     true,
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) {
				return NewAdaptiveOrderedLocalQueue(size, defaultLocalQueueCapcity), nil
			},
		},
		{
			Name:                "LocalOrdered",
			OrderedStartsBefore: false,
			OrderedSupported:    true,
			Constructor:         func(ctx context.Context, size int) (amboy.Queue, error) { return NewLocalOrdered(size), nil },
		},
		{
			Name: "Priority",
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) {
				return NewLocalPriorityQueue(size, defaultLocalQueueCapcity), nil
			},
		},
		{
			Name:                    "LimitedSize",
			WaitUntilSupported:      true,
			DispatchBeforeSupported: true,
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) {
				return NewLocalLimitedSize(size, 1024*size), nil
			},
		},
		{
			Name: "Shuffled",
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) {
				return NewShuffledLocal(size, defaultLocalQueueCapcity), nil
			},
		},
		{
			Name:        "RemoteUnordered",
			IsRemote:    true,
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) { return NewRemoteUnordered(size), nil },
		},
		{
			Name:             "RemoteOrdered",
			IsRemote:         true,
			OrderedSupported: true,
			Constructor:      func(ctx context.Context, size int) (amboy.Queue, error) { return NewSimpleRemoteOrdered(size), nil },
		},
		{
			Name:    "SQSFifo",
			MaxSize: 4,
			Skip:    true,
			Constructor: func(ctx context.Context, size int) (amboy.Queue, error) {
				return NewSQSFifoQueue(randomString(4), size)
			},
		},
	}
}

func DefaultDriverTestCases(client *mongo.Client, session *mgo.Session) []DriverTestCase {
	return []DriverTestCase{
		{
			Name:          "No",
			SupportsLocal: true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				return nil, func(_ context.Context) error { return nil }, errors.New("not supported")
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				return func(_ context.Context) error { return nil }, nil
			},
		},
		{
			Name: "Internal",
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				return nil, func(_ context.Context) error { return nil }, errors.New("not supported")
			},
			SkipOrdered: true,
			Skip:        true,
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				d := NewInternalDriver()
				closer := func(ctx context.Context) error {
					d.Close()
					return nil
				}

				return closer, remote.SetDriver(d)
			},
		},
		{
			Name: "Priority",
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				return nil, func(_ context.Context) error { return nil }, errors.New("not supported")
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				d := NewPriorityDriver()
				closer := func(ctx context.Context) error {
					d.Close()
					return nil
				}

				return closer, remote.SetDriver(d)
			},
		},
		{
			Name:               "Mgo",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"

				var err error
				out := make([]Driver, size)
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					out[i], err = OpenNewMgoDriver(ctx, name, opts, session.Clone())
					catcher.Add(err)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mgoDriver).Close()
						}
					}
					return session.DB(opts.DB).C(addJobsSuffix(name)).DropCollection()
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"

				driver, err := OpenNewMgoDriver(ctx, name, opts, session.Clone())
				if err != nil {
					return nil, err
				}

				d := driver.(*mgoDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return session.DB(opts.DB).C(addJobsSuffix(name)).DropCollection()
				}

				return closer, remote.SetDriver(d)
			},
		},
		{
			Name:               "Mongo",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.Format = amboy.BSON2

				var err error
				out := make([]Driver, size)
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					out[i], err = OpenNewMongoDriver(ctx, name, opts, client)
					catcher.Add(err)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mongoDriver).Close()
						}
					}
					return client.Database(opts.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"

				driver, err := OpenNewMongoDriver(ctx, name, opts, client)
				if err != nil {
					return nil, err
				}

				d := driver.(*mongoDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return client.Database(opts.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return closer, remote.SetDriver(d)

			},
		},
		{
			Name:               "MongoGroup",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.Format = amboy.BSON2

				out := []Driver{}
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					driver, err := OpenNewMongoGroupDriver(ctx, name, opts, name+"one", client)
					catcher.Add(err)
					out = append(out, driver)
					driver, err = OpenNewMongoGroupDriver(ctx, name, opts, name+"two", client)
					catcher.Add(err)
					out = append(out, driver)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mongoGroupDriver).Close()
						}
					}
					return client.Database(opts.DB).Collection(addGroupSufix(name)).Drop(ctx)
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.CheckWaitUntil = true

				driver, err := OpenNewMongoGroupDriver(ctx, name, opts, name+"three", client)
				if err != nil {
					return nil, err
				}

				d := driver.(*mongoGroupDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return client.Database(opts.DB).Collection(addGroupSufix(name)).Drop(ctx)
				}

				return closer, remote.SetDriver(driver)
			},
		},
		{
			Name:               "MgoGroup",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"

				out := []Driver{}
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					driver, err := OpenNewMgoGroupDriver(ctx, name, opts, name+"four", session.Clone())
					catcher.Add(err)
					out = append(out, driver)
					driver, err = OpenNewMgoGroupDriver(ctx, name, opts, name+"five", session.Clone())
					catcher.Add(err)
					out = append(out, driver)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mgoGroupDriver).Close()
						}
					}
					return session.DB(opts.DB).C(addGroupSufix(name)).DropCollection()
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"

				driver, err := OpenNewMgoGroupDriver(ctx, name, opts, name+"six", session.Clone())
				if err != nil {
					return nil, err
				}

				d := driver.(*mgoGroupDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return session.DB(opts.DB).C(addGroupSufix(name)).DropCollection()
				}

				return closer, remote.SetDriver(driver)
			},
		},
		{
			Name:               "MongoMGOBSON",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.Format = amboy.BSON

				var err error
				out := make([]Driver, size)
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					out[i], err = OpenNewMongoDriver(ctx, name, opts, client)
					catcher.Add(err)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mongoDriver).Close()
						}
					}
					return client.Database(opts.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.Format = amboy.BSON

				driver, err := OpenNewMongoDriver(ctx, name, opts, client)
				if err != nil {
					return nil, err
				}

				d := driver.(*mongoDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return client.Database(opts.DB).Collection(addJobsSuffix(name)).Drop(ctx)
				}

				return closer, remote.SetDriver(d)

			},
		},
		{
			Name:               "MongoGroupMGOBSON",
			WaitUntilSupported: true,
			SupportsMulti:      true,
			Constructor: func(ctx context.Context, name string, size int) ([]Driver, TestCloser, error) {
				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.Format = amboy.BSON

				out := []Driver{}
				catcher := grip.NewBasicCatcher()
				for i := 0; i < size; i++ {
					driver, err := OpenNewMongoGroupDriver(ctx, name, opts, name+"one", client)
					catcher.Add(err)
					out = append(out, driver)
					driver, err = OpenNewMongoGroupDriver(ctx, name, opts, name+"two", client)
					catcher.Add(err)
					out = append(out, driver)
				}
				closer := func(ctx context.Context) error {
					for _, d := range out {
						if d != nil {
							d.(*mongoGroupDriver).Close()
						}
					}
					return client.Database(opts.DB).Collection(addGroupSufix(name)).Drop(ctx)
				}

				return out, closer, catcher.Resolve()
			},
			SetDriver: func(ctx context.Context, q amboy.Queue, name string) (TestCloser, error) {
				remote, ok := q.(Remote)
				if !ok {
					return nil, errors.New("invalid queue type")
				}

				opts := DefaultMongoDBOptions()
				opts.DB = "amboy_test"
				opts.CheckWaitUntil = true
				opts.Format = amboy.BSON

				driver, err := OpenNewMongoGroupDriver(ctx, name, opts, name+"three", client)
				if err != nil {
					return nil, err
				}

				d := driver.(*mongoGroupDriver)
				closer := func(ctx context.Context) error {
					d.Close()
					return client.Database(opts.DB).Collection(addGroupSufix(name)).Drop(ctx)
				}

				return closer, remote.SetDriver(driver)
			},
		},
	}
}

func DefaultPoolTestCases() []PoolTestCase {
	return []PoolTestCase{
		{
			Name:    "Default",
			SetPool: func(q amboy.Queue, _ int) error { return nil },
		},
		{
			Name:      "Single",
			SkipMulti: true,
			MaxSize:   1,
			SetPool: func(q amboy.Queue, _ int) error {
				runner := pool.NewSingle()
				if err := runner.SetQueue(q); err != nil {
					return err
				}

				return q.SetRunner(runner)
			},
		},
		{
			Name:    "Abortable",
			MinSize: 4,
			SetPool: func(q amboy.Queue, size int) error { return q.SetRunner(pool.NewAbortablePool(size, q)) },
		},
		{
			Name:    "RateLimitedSimple",
			MinSize: 4,
			MaxSize: 16,
			SetPool: func(q amboy.Queue, size int) error {
				runner, err := pool.NewSimpleRateLimitedWorkers(size, 10*time.Millisecond, q)
				if err != nil {
					return nil
				}

				return q.SetRunner(runner)
			},
		},
		{
			Name:       "RateLimitedAverage",
			MinSize:    4,
			MaxSize:    16,
			SkipMulti:  true,
			SkipRemote: true,
			SetPool: func(q amboy.Queue, size int) error {
				runner, err := pool.NewMovingAverageRateLimitedWorkers(size, size*100, 10*time.Millisecond, q)
				if err != nil {
					return nil
				}

				return q.SetRunner(runner)
			},
		},
	}
}

func DefaultSizeTestCases() []SizeTestCase {
	return []SizeTestCase{
		{Name: "One", Size: 1},
		{Name: "Two", Size: 2},
		{Name: "Four", Size: 4},
		{Name: "Eight", Size: 8},
		{Name: "Sixteen", Size: 16},
		{Name: "ThirtyTwo", Size: 32},
		{Name: "SixtyFour", Size: 64},
	}
}

func DefaultMultipleExecututionTestCases(driver DriverTestCase) []MultipleExecutionTestCase {
	return []MultipleExecutionTestCase{
		{
			Name:            "MultiTwoDrivers",
			MultipleDrivers: true,
			Setup: func(ctx context.Context, qOne, qTwo amboy.Queue) (TestCloser, error) {
				catcher := grip.NewBasicCatcher()
				driverID := newDriverID()
				dcloserOne, err := driver.SetDriver(ctx, qOne, driverID)
				catcher.Add(err)

				dcloserTwo, err := driver.SetDriver(ctx, qTwo, driverID)
				catcher.Add(err)

				closer := func(ctx context.Context) error {
					err := dcloserOne(ctx)
					grip.Info(dcloserTwo(ctx))
					return err
				}

				return closer, catcher.Resolve()
			},
		},
		{
			Name: "MultiOneDriver",
			Setup: func(ctx context.Context, qOne, qTwo amboy.Queue) (TestCloser, error) {
				catcher := grip.NewBasicCatcher()
				driverID := newDriverID()
				dcloserOne, err := driver.SetDriver(ctx, qOne, driverID)
				catcher.Add(err)

				rq := qOne.(Remote)
				catcher.Add(qTwo.(Remote).SetDriver(rq.Driver()))

				return dcloserOne, catcher.Resolve()
			},
		},
	}

}

func TestQueueSmoke(t *testing.T) {
	bctx, bcancel := context.WithCancel(context.Background())
	defer bcancel()

	session, err := mgo.DialWithTimeout("mongodb://localhost:27017", time.Second)
	require.NoError(t, err)
	defer session.Close()

	client, err := mongo.NewClient(options.Client().ApplyURI("mongodb://localhost:27017").SetConnectTimeout(time.Second))
	require.NoError(t, err)
	require.NoError(t, client.Connect(bctx))

	defer func() { require.NoError(t, client.Disconnect(bctx)) }()

	for _, test := range DefaultQueueTestCases() {
		if test.Skip {
			continue
		}

		t.Run(test.Name, func(t *testing.T) {
			for _, driver := range DefaultDriverTestCases(client, session) {
				if driver.Skip {
					continue
				}

				if test.IsRemote == driver.SupportsLocal {
					continue
				}
				if test.OrderedSupported && driver.SkipOrdered {
					continue
				}

				t.Run(driver.Name+"Driver", func(t *testing.T) {
					for _, runner := range DefaultPoolTestCases() {
						if test.IsRemote && runner.SkipRemote {
							continue
						}

						t.Run(runner.Name+"Pool", func(t *testing.T) {
							for _, size := range DefaultSizeTestCases() {
								if test.MaxSize > 0 && size.Size > test.MaxSize {
									continue
								}

								if runner.MinSize > 0 && runner.MinSize > size.Size {
									continue
								}

								if runner.MaxSize > 0 && runner.MaxSize < size.Size {
									continue
								}

								t.Run(size.Name, func(t *testing.T) {
									if !test.SkipUnordered {
										t.Run("Unordered", func(t *testing.T) {
											UnorderedTest(bctx, t, test, driver, runner, size)
										})
									}
									if test.OrderedSupported && !driver.SkipOrdered {
										t.Run("Ordered", func(t *testing.T) {
											OrderedTest(bctx, t, test, driver, runner, size)
										})
									}
									if test.WaitUntilSupported || driver.WaitUntilSupported {
										t.Run("WaitUntil", func(t *testing.T) {
											WaitUntilTest(bctx, t, test, driver, runner, size)
										})
									}

									if test.DispatchBeforeSupported || driver.DispatchBeforeSupported {
										t.Run("DispatchBefore", func(t *testing.T) {
											DispatchBeforeTest(bctx, t, test, driver, runner, size)
										})
									}

									t.Run("OneExecution", func(t *testing.T) {
										OneExecutionTest(bctx, t, test, driver, runner, size)
									})

									if test.IsRemote && driver.SupportsMulti && !runner.SkipMulti {
										for _, multi := range DefaultMultipleExecututionTestCases(driver) {
											if multi.MultipleDrivers && size.Name != "Single" {
												continue
											}
											t.Run(multi.Name, func(t *testing.T) {
												MultiExecutionTest(bctx, t, test, driver, runner, size, multi)
											})
										}

										if size.Size < 8 {
											t.Run("ManyQueues", func(t *testing.T) {
												ManyQueueTest(bctx, t, test, driver, runner, size)
											})
										}
									}
								})
							}
						})
					}
				})
			}
		})
	}
}

func UnorderedTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()

	q, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))
	dcloser, err := driver.SetDriver(ctx, q, newDriverID())
	require.NoError(t, err)
	defer func() { require.NoError(t, dcloser(ctx)) }()

	if test.OrderedSupported && !test.OrderedStartsBefore {
		// pass
	} else {
		require.NoError(t, q.Start(ctx))
	}

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false", ""}
	numJobs := size.Size * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < size.Size; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d", name, num)
				j := job.NewShellJob(cmd, "")
				assert.NoError(t, q.Put(ctx, j),
					fmt.Sprintf("with %d workers", num))
				_, ok := q.Get(ctx, j.ID())
				assert.True(t, ok)
			}
		}(i)
	}
	wg.Wait()
	if test.OrderedSupported && !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)

	assert.Equal(t, numJobs, q.Stats(ctx).Total, fmt.Sprintf("with %d workers", size.Size))

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)

	grip.Infof("workers complete for %d worker smoke test", size.Size)
	assert.Equal(t, numJobs, q.Stats(ctx).Completed, fmt.Sprintf("%+v", q.Stats(ctx)))
	for result := range q.Results(ctx) {
		assert.True(t, result.Status().Completed, fmt.Sprintf("with %d workers", size.Size))

		// assert that we had valid time info persisted
		ti := result.TimeInfo()
		assert.NotZero(t, ti.Start)
		assert.NotZero(t, ti.End)
	}

	statCounter := 0
	for stat := range q.JobStats(ctx) {
		statCounter++
		assert.True(t, stat.ID != "")
	}
	assert.Equal(t, numJobs, statCounter, fmt.Sprintf("want jobStats for every job"))

	grip.Infof("completed results check for %d worker smoke test", size.Size)
}

func OrderedTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()

	q, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))

	dcloser, err := driver.SetDriver(ctx, q, newDriverID())
	require.NoError(t, err)
	defer func() { require.NoError(t, dcloser(ctx)) }()

	var lastJobName string

	testNames := []string{"amboy", "cusseta", "jasper", "sardis", "dublin"}

	numJobs := size.Size / 2 * len(testNames)

	tempDir, err := ioutil.TempDir("", strings.Join([]string{"amboy-ordered-queue-smoke-test",
		uuid.NewV4().String()}, "-"))
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	if test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}
	for i := 0; i < size.Size/2; i++ {
		for _, name := range testNames {
			fn := filepath.Join(tempDir, fmt.Sprintf("%s.%d", name, i))
			cmd := fmt.Sprintf("echo %s", fn)
			j := job.NewShellJob(cmd, fn)
			if lastJobName != "" {
				require.NoError(t, j.Dependency().AddEdge(lastJobName))
			}
			lastJobName = j.ID()

			require.NoError(t, q.Put(ctx, j))
		}
	}

	if !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	require.Equal(t, numJobs, q.Stats(ctx).Total, fmt.Sprintf("with %d workers", size.Size))
	amboy.WaitInterval(ctx, q, 50*time.Millisecond)
	require.Equal(t, numJobs, q.Stats(ctx).Completed, fmt.Sprintf("%+v", q.Stats(ctx)))
	for result := range q.Results(ctx) {
		require.True(t, result.Status().Completed, fmt.Sprintf("with %d workers", size.Size))
	}

	statCounter := 0
	for stat := range q.JobStats(ctx) {
		statCounter++
		require.True(t, stat.ID != "")
	}
	require.Equal(t, statCounter, numJobs)
}

func WaitUntilTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))

	dcloser, err := driver.SetDriver(ctx, q, newDriverID())
	require.NoError(t, err)
	defer func() { require.NoError(t, dcloser(ctx)) }()

	require.NoError(t, q.Start(ctx))

	testNames := []string{"test", "second", "workers", "forty-two", "true", "false"}

	sz := size.Size
	if sz > 16 {
		sz = 16
	} else if sz < 2 {
		sz = 2
	}

	numJobs := sz * len(testNames)

	wg := &sync.WaitGroup{}

	for i := 0; i < sz; i++ {
		wg.Add(1)
		go func(num int) {
			defer wg.Done()
			for _, name := range testNames {
				cmd := fmt.Sprintf("echo %s.%d.default", name, num)
				j := job.NewShellJob(cmd, "")
				ti := j.TimeInfo()
				require.Zero(t, ti.WaitUntil)
				require.NoError(t, q.Put(ctx, j), fmt.Sprintf("(a) with %d workers", num))
				_, ok := q.Get(ctx, j.ID())
				require.True(t, ok)

				cmd = fmt.Sprintf("echo %s.%d.waiter", name, num)
				j2 := job.NewShellJob(cmd, "")
				j2.UpdateTimeInfo(amboy.JobTimeInfo{
					WaitUntil: time.Now().Add(time.Hour),
				})
				ti2 := j2.TimeInfo()
				require.NotZero(t, ti2.WaitUntil)
				require.NoError(t, q.Put(ctx, j2), fmt.Sprintf("(b) with %d workers", num))
				_, ok = q.Get(ctx, j2.ID())
				require.True(t, ok)
			}
		}(i)
	}
	wg.Wait()

	// waitC for things to finish
	const (
		interval = 100 * time.Millisecond
		maxTime  = 3 * time.Second
	)
	var dur time.Duration
	timer := time.NewTimer(interval)
	defer timer.Stop()
waitLoop:
	for {
		select {
		case <-ctx.Done():
			break waitLoop
		case <-timer.C:
			dur += interval
			stat := q.Stats(ctx)
			if stat.Completed >= numJobs {
				break waitLoop
			}

			if dur >= maxTime {
				break waitLoop
			}

			timer.Reset(interval)
		}
	}

	stats := q.Stats(ctx)
	require.Equal(t, numJobs*2, stats.Total)
	assert.Equal(t, numJobs, stats.Completed)

	completed := 0
	for result := range q.Results(ctx) {
		status := result.Status()
		ti := result.TimeInfo()

		if status.Completed {
			completed++
			require.True(t, ti.WaitUntil.IsZero(), "val=%s id=%s", ti.WaitUntil, result.ID())
		} else {
			require.False(t, ti.WaitUntil.IsZero(), "val=%s id=%s", ti.WaitUntil, result.ID())
		}
	}

	assert.Equal(t, numJobs, completed)
}

func DispatchBeforeTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))

	dcloser, err := driver.SetDriver(ctx, q, newDriverID())
	require.NoError(t, err)
	defer func() { require.NoError(t, dcloser(ctx)) }()

	require.NoError(t, q.Start(ctx))

	for i := 0; i < 2*size.Size; i++ {
		j := job.NewShellJob("ls", "")
		ti := j.TimeInfo()

		if i%2 == 0 {
			ti.DispatchBy = time.Now().Add(time.Second)
		} else {
			ti.DispatchBy = time.Now().Add(-time.Second)
		}
		j.UpdateTimeInfo(ti)
		require.NoError(t, q.Put(ctx, j))
	}

	time.Sleep(100 * time.Millisecond)
	stats := q.Stats(ctx)
	assert.Equal(t, 2*size.Size, stats.Total)
	assert.Equal(t, size.Size, stats.Completed)
}

func OneExecutionTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	if test.Name == "LocalOrdered" {
		t.Skip("topological sort deadlocks")
	}
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	q, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(q, size.Size))

	dcloser, err := driver.SetDriver(ctx, q, newDriverID())
	require.NoError(t, err)
	defer func() { require.NoError(t, dcloser(ctx)) }()

	mockJobCounters.Reset()
	count := 40

	if !test.OrderedSupported || test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	for i := 0; i < count; i++ {
		j := newMockJob()
		jobID := fmt.Sprintf("%d.%d.mock.single-exec", i, job.GetNumber())
		j.SetID(jobID)
		assert.NoError(t, q.Put(ctx, j))
	}

	if test.OrderedSupported && !test.OrderedStartsBefore {
		require.NoError(t, q.Start(ctx))
	}

	amboy.WaitInterval(ctx, q, 100*time.Millisecond)
	assert.Equal(t, count, mockJobCounters.Count())
}

func MultiExecutionTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase, multi MultipleExecutionTestCase) {
	ctx, cancel := context.WithTimeout(bctx, 2*time.Minute)
	defer cancel()

	qOne, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	qTwo, err := test.Constructor(ctx, size.Size)
	require.NoError(t, err)
	require.NoError(t, runner.SetPool(qOne, size.Size))
	require.NoError(t, runner.SetPool(qTwo, size.Size))

	closer, err := multi.Setup(ctx, qOne, qTwo)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()

	assert.NoError(t, qOne.Start(ctx))
	assert.NoError(t, qTwo.Start(ctx))

	num := 200
	adderProcs := 4

	wg := &sync.WaitGroup{}
	for o := 0; o < adderProcs; o++ {
		wg.Add(1)
		go func(o int) {
			defer wg.Done()
			// add a bunch of jobs: half to one queue and half to the other.
			for i := 0; i < num; i++ {
				cmd := fmt.Sprintf("echo %d.%d", o, i)
				j := job.NewShellJob(cmd, "")
				if i%2 == 0 {
					assert.NoError(t, qOne.Put(ctx, j))
				} else {
					assert.NoError(t, qTwo.Put(ctx, j))
				}

			}
		}(o)
	}
	wg.Wait()

	num = num * adderProcs

	grip.Info("added jobs to queues")

	// wait for all jobs to complete.
	amboy.WaitInterval(ctx, qOne, 100*time.Millisecond)
	amboy.WaitInterval(ctx, qTwo, 100*time.Millisecond)

	// check that both queues see all jobs
	statsOne := qOne.Stats(ctx)
	statsTwo := qTwo.Stats(ctx)

	var shouldExit bool
	if !assert.Equal(t, num, statsOne.Total, "ONE: %+v", statsOne) {
		shouldExit = true
	}
	if !assert.Equal(t, num, statsTwo.Total, "TWO: %+v", statsTwo) {
		shouldExit = true
	}
	if shouldExit {
		return
	}

	// check that all the results in the queues are are completed,
	// and unique
	firstCount := 0
	results := make(map[string]struct{})
	for result := range qOne.Results(ctx) {
		firstCount++
		assert.True(t, result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	secondCount := 0
	// make sure that all of the results in the second queue match
	// the results in the first queue.
	for result := range qTwo.Results(ctx) {
		secondCount++
		assert.True(t, result.Status().Completed)
		results[result.ID()] = struct{}{}
	}

	assert.Equal(t, firstCount, secondCount)
	assert.Equal(t, len(results), firstCount)
}

func ManyQueueTest(bctx context.Context, t *testing.T, test QueueTestCase, driver DriverTestCase, runner PoolTestCase, size SizeTestCase) {
	ctx, cancel := context.WithCancel(bctx)
	defer cancel()
	driverID := newDriverID()

	sz := size.Size
	if sz > 8 {
		sz = 8
	} else if sz < 2 {
		sz = 2
	}

	drivers, closer, err := driver.Constructor(ctx, driverID, sz)
	require.NoError(t, err)
	defer func() { require.NoError(t, closer(ctx)) }()

	queues := []Remote{}
	for _, driver := range drivers {
		q, err := test.Constructor(ctx, size.Size)
		require.NoError(t, err)
		queue := q.(Remote)

		require.NoError(t, queue.SetDriver(driver))
		require.NoError(t, q.Start(ctx))
		queues = append(queues, queue)
	}

	const (
		inside  = 15
		outside = 10
	)

	mockJobCounters.Reset()
	wg := &sync.WaitGroup{}
	for i := 0; i < len(drivers); i++ {
		for ii := 0; ii < outside; ii++ {
			wg.Add(1)
			go func(f, s int) {
				defer wg.Done()
				for iii := 0; iii < inside; iii++ {
					j := newMockJob()
					j.SetID(fmt.Sprintf("%d-%d-%d-%d", f, s, iii, job.GetNumber()))
					assert.NoError(t, queues[0].Put(ctx, j))
				}
			}(i, ii)
		}
	}

	grip.Notice("waiting to add all jobs")
	wg.Wait()

	grip.Notice("waiting to run jobs")

	for _, q := range queues {
		amboy.WaitInterval(ctx, q, 20*time.Millisecond)
	}

	assert.Equal(t, len(drivers)*inside*outside, mockJobCounters.Count())
}
